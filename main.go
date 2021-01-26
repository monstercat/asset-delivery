package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	s3util "github.com/monstercat/golib/s3"
	"github.com/nfnt/resize"
)

const MaxImageDimension = 4096

var ErrFileNotHandled = errors.New("file type not handled")
var ErrInvalidBounds = errors.New("invalid image bounds")

type Service struct {
	Session *session.Session
	Bucket string
	AssetDir string
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

func main () {
	s := &Service{}
	var address string
	flag.StringVar(&address, "address", "0.0.0.0:80", "The binding address for the application.")
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
		fmt.Printf("aws session error: %s\n", err.Error())
		os.Exit(1)
		return
	}

	log.Println("Opening HTTP server")
	err = http.ListenAndServe(address, s)
	if err != nil {
		fmt.Printf("issue opening server: %s\n", err.Error())
		os.Exit(1)
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

	writeError := func (status int, msg string) {
		w.WriteHeader(status)
		fmt.Fprintf(w, msg)
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

func clearCache(s *Service, filename string, size uint64) error {
	return s3util.DeleteS3(s, path.Join(strconv.Itoa(int(size)), filename))
}

func cacheAndServe(s *Service, filename string, size uint64) (string, error) {
	key := path.Join(strconv.Itoa(int(size)), filename)
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
	ext := strings.ToLower(filepath.Ext(p))
	var fn func(io.Reader) (image.Image, error)
	switch ext {
	case ".jpeg", ".jfif", ".jpg":
		fn = jpeg.Decode
	case ".png":
		fn = png.Decode
	}
	if fn == nil {
		return nil, ErrFileNotHandled
	}
	return fn(f)
}