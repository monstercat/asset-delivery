package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"math/rand"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/chai2010/webp"
	s3util "github.com/monstercat/golib/s3"
	stringutil "github.com/monstercat/golib/string"
	"github.com/nats-io/nats.go"
	"github.com/nfnt/resize"
)

const MaxImageDimension = 4096
const NatsSubjectAssetDelivery = "asset-delivery"

var ErrFileNotHandled = errors.New("file type not handled")
var ErrInvalidBounds = errors.New("invalid image bounds")
var ErrWidthZero = errors.New("width cannot be zero")

type VideoInfo struct {
	Filepath string
	Width    uint64
}

func (v VideoInfo) Filename() string {
	return filepath.Base(v.Filepath)
}

func (v VideoInfo) GetS3Key() string {
	// Since video encoding is always webm, have to return the filename with webm extension.
	withoutExtension := strings.TrimSuffix(v.Filename(), filepath.Ext(v.Filename()))
	ext := ".webm"
	filename := withoutExtension + ext
	return getResizedKey(filename, v.Width)
}

type VideoQueue struct {
	lock    sync.Mutex
	pending []VideoInfo
}

func (q *VideoQueue) Enqueue(f VideoInfo) {
	q.lock.Lock()
	defer q.lock.Unlock()
	q.pending = append(q.pending, f)
}

func (q *VideoQueue) Dequeue() *VideoInfo {
	q.lock.Lock()
	defer q.lock.Unlock()
	if len(q.pending) == 0 {
		return nil
	}
	first := &q.pending[0]
	q.pending = q.pending[1:]
	return first
}

type Service struct {
	Session   *session.Session
	Queue     *VideoQueue
	Bucket    string
	AssetDir  string
	ClearCode string
}

func (s *Service) GetSession() *session.Session {
	return s.Session
}

func (s *Service) DefaultBucket() string {
	return s.Bucket
}

func (s *Service) MkHashKey(key string) string {
	return key
}

func (s *Service) MkURL(key string) string {
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", s.DefaultBucket(), key)
}

func (s *Service) GetAssetPath(str string) string {
	return filepath.Join(s.AssetDir, str)
}

func main() {
	s := &Service{}
	var address string
	var natsURL string
	flag.StringVar(&address, "address", "0.0.0.0:80", "The binding address for the application.")
	flag.StringVar(&natsURL, "nats", "nats://127.0.0.1:4222", "The url to the nats server.")
	flag.StringVar(&s.Bucket, "bucket", "", "S3 bucket to put cached images")
	flag.StringVar(&s.AssetDir, "dir", ".", "The asset directory to serve")
	flag.StringVar(&s.ClearCode, "cache-code", "itscooltorefreshwithbudlight", "The code to use to delete a cached resized image.")
	flag.Parse()

	var err error
	awsConfig := aws.NewConfig().
		WithRegion(os.Getenv("AWS_DEFAULT_REGION")).
		WithCredentials(credentials.NewEnvCredentials())
	s.Session, err = session.NewSession(awsConfig)
	if err != nil {
		log.Fatalf("aws session error: %v\n", err)
	}

	q := &VideoQueue{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go processQueue(ctx, s, q)
	s.Queue = q

	nc, err := nats.Connect(natsURL, func(opts *nats.Options) error {
		opts.AllowReconnect = true
		opts.MaxReconnect = -1
		opts.ReconnectWait = time.Second * 2
		opts.Timeout = time.Second * 5
		opts.Name = "asset-delivery"
		return nil
	})
	if err != nil {
		log.Fatalf("nats connection error: %v\n", err)
	}
	defer nc.Close()
	sub, err := nc.QueueSubscribe(NatsSubjectAssetDelivery, "q", mkNatsHandler(q))
	if err != nil {
		log.Fatalf("unable to subscribe to nats %v\n", err)
	}
	defer sub.Unsubscribe()

	log.Println("Opening HTTP server")
	err = http.ListenAndServe(address, s)
	if err != nil {
		log.Fatalf("issue opening server: %v\n", err)
	}
}

func mkNatsHandler(q *VideoQueue) func(msg *nats.Msg) {
	return func(msg *nats.Msg) {
		var data VideoInfo
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			log.Println("Unable to unmarshal VideoInfo from nats message", err)
			return
		}
		q.Enqueue(data)
	}
}

func parseSize(str string) (uint64, error) {
	size, err := strconv.ParseUint(str, 10, 32)
	if err != nil {
		return 0, errors.New("bad image size provided")
	}
	if size > MaxImageDimension {
		return 0, errors.New(fmt.Sprintf(`image size cannot exceed %d`, MaxImageDimension))
	}
	return size, nil
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, "GET methods only.")
		return
	}

	logError := func(err error) {
		log.Printf("request error: [%s] %s\n", r.URL.String(), err.Error())
	}

	writeError := func(status int, msg string) {
		w.WriteHeader(status)
		fmt.Fprintf(w, msg)
	}

	if isVideoFile(r) {
		serveVideo(s, w, r)
		return
	}

	// Let's handle resize of local files
	query := r.URL.Query()
	imageWidth := query.Get("image_width")
	if imageWidth != "" {
		size, err := parseSize(imageWidth)
		if err != nil {
			writeError(http.StatusBadRequest, err.Error())
			logError(err)
			return
		}

		if v := query.Get("clear"); v == s.ClearCode {
			if err := clearCache(s, r.URL.Path, size); err != nil {
				writeError(http.StatusInternalServerError, "An issue occurred while attempting the operation.")
				logError(err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		} else if v != "" {
			writeError(http.StatusUnauthorized, "Not authorized to perform that action.")
			return
		}

		url, err := cacheAndServe(s, r.URL.Path, size)
		if os.IsNotExist(err) {
			writeError(http.StatusNotFound, "Not found.")
			return
		} else if err != nil {
			writeError(http.StatusInternalServerError, "An error occurred.")
			logError(err)
			return
		}
		http.Redirect(w, r, url, http.StatusFound)
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func serveVideo(s *Service, w http.ResponseWriter, r *http.Request) {
	writeError := func(status int, msg string) {
		w.WriteHeader(status)
		fmt.Fprintf(w, msg)
	}

	query := r.URL.Query()
	width, err := strconv.Atoi(query.Get("width"))
	if err != nil {
		writeError(http.StatusInternalServerError, err.Error())
		return
	}
	if width == 0 {
		writeError(http.StatusBadRequest, ErrWidthZero.Error())
		return
	}

	video := VideoInfo{
		Filepath: s.GetAssetPath(r.URL.Path),
		Width:    uint64(width),
	}

	key := video.GetS3Key()
	_, ok, err := s3util.ObjectExistsS3(s, key)
	if err != nil {
		writeError(http.StatusInternalServerError, err.Error())
		return
	} else if ok {
		http.Redirect(w, r, s.MkURL(key), http.StatusFound)
		return
	}
	s.Queue.Enqueue(video)

	w.WriteHeader(http.StatusNotFound)
}

func getResizedKey(filename string, size uint64) string {
	return path.Join(strconv.Itoa(int(size)), filename)
}

func isVideoFile(r *http.Request) bool {
	mimeTypes := []string{
		"video/quicktime",
		"video/mp4",
		"video/webm",
	}
	suffixes := []string{
		".mp4",
		".mov",
		".webm",
	}
	if stringutil.StringInList(mimeTypes, strings.ToLower(r.Header.Get("content-type"))) {
		return true
	}
	for _, suffix := range suffixes {
		if strings.HasSuffix(strings.ToLower(r.URL.Path), suffix) {
			return true
		}
	}
	return false
}

func clearCache(s *Service, filename string, size uint64) error {
	return s3util.DeleteS3(s, getResizedKey(filename, size))
}

func cacheAndServe(s *Service, filename string, size uint64) (string, error) {
	key := getResizedKey(filename, size)
	_, ok, err := s3util.ObjectExistsS3(s, key)
	if err != nil {
		return "", err
	} else if ok {
		return s.MkURL(key), nil
	}
	img, err := OpenImage(s.GetAssetPath(filename))
	if err != nil {
		return "", err
	}
	bounds := img.Bounds()
	width := bounds.Max.X
	height := bounds.Max.Y
	if bounds.Max.X <= 0 {
		return "", ErrInvalidBounds
	}
	ratio := float64(bounds.Max.Y) / float64(bounds.Max.X)
	height = int(float64(size) * ratio)
	width = int(size)
	img = resize.Resize(uint(width), uint(height), img, resize.Bicubic)
	buf, err := ImageToBytes(img, filepath.Ext(filename))
	if err != nil {
		return "", err
	}
	err = s3util.UploadS3(s, buf, mime.TypeByExtension(filepath.Ext(filename)), s3.BucketCannedACLPublicRead, key)
	return s.MkURL(key), err
}

func ImageToBytes(i image.Image, ext string) (*bytes.Buffer, error) {
	buf := bytes.NewBuffer([]byte{})
	var err error
	switch ext {
	case ".jpeg", ".jfif", ".jpg":
		err = jpeg.Encode(buf, i, &jpeg.Options{Quality: 100})
	case ".png":
		err = png.Encode(buf, i)
	case ".webp":
		err = webp.Encode(buf, i, &webp.Options{Quality: 100})
	default:
		err = ErrFileNotHandled
	}
	return buf, err
}

func OpenImage(p string) (image.Image, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	ext := strings.ToLower(filepath.Ext(p))
	var fn func(io.Reader) (image.Image, error)
	switch ext {
	case ".jpeg", ".jfif", ".jpg":
		fn = jpeg.Decode
	case ".png":
		fn = png.Decode
	case ".webp":
		fn = webp.Decode
	}
	if fn == nil {
		return nil, ErrFileNotHandled
	}
	return fn(f)
}

func processQueue(ctx context.Context, info s3util.S3Info, q *VideoQueue) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.Tick(time.Second):
			vid := q.Dequeue()
			if vid == nil {
				break
			}
			if err := processVideo(ctx, info, vid); err != nil {
				log.Printf("Unable to process video %+v: %v\n", vid, err)
				// return err // continue processing??
			}
		}
	}
	return nil
}

func processVideo(ctx context.Context, info s3util.S3Info, f *VideoInfo) error {
	key := f.GetS3Key()
	_, ok, err := s3util.ObjectExistsS3(info, key)
	if err != nil {
		return err
	} else if ok {
		// already processed
		return nil
	}

	srcPath := f.Filepath
	if _, err := os.Stat(srcPath); err != nil {
		return err
	}

	dstPath := path.Join(os.TempDir(), fmt.Sprintf("%v%v", rand.Int63(), f.Filename()))
	defer os.Remove(dstPath)

	log.Printf("Encoding %v\n", f.Filepath)
	args := []string{
		"-i", srcPath,
		// The range of the CRF scale is 0–51, where 0 is lossless,
		// 23 is the default, and 51 is worst quality possible.
		// A lower value generally leads to higher quality, and a subjectively sane range is 17–28.
		"-crf", "17",
		"-y", // overwrite
		"-vf", fmt.Sprintf("scale=%v:-1", f.Width),
		dstPath,
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	dst, err := os.Open(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	log.Printf("Uploading %v\n", f.Filepath)
	if err := s3util.UploadS3(info, dst, mime.TypeByExtension(filepath.Ext(f.Filename())), s3.BucketCannedACLPublicRead, key); err != nil {
		return err
	}
	log.Printf("Completed %v\n", f.Filepath)
	return nil
}
