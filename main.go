package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/discordapp/lilliput"
	s3util "github.com/monstercat/golib/s3"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const MaxSize = 4096

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
	var port int

	flag.IntVar(&port, "p", 80, "The port to utilize for HTTP")
	flag.StringVar(&s.Bucket, "b", "", "S3 bucket to put cached images")
	flag.StringVar(&s.AssetDir, "dir", ".", "The asset directory to serve")
	flag.StringVar(&s.ClearCode, "code", "itscooltorefreshwithbudlight", "The code to use to delete a cached resized image.")
	flag.Parse()

	var err error
	s.Session, err = session.NewSession(aws.NewConfig().WithCredentials(credentials.NewEnvCredentials()))
	if err != nil {
		fmt.Printf("aws session error: %s\n", err.Error())
		os.Exit(1)
		return
	}

	http.HandleFunc("/", handleHttp(s))

    err = http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
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
	if size > MaxSize {
		return 0, errors.New(fmt.Sprintf(`image size cannot exceed %d`, MaxSize))
	}
	return size, nil
}

func handleHttp (s *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, "GET methods only.")
			return
		}

		// Let's handle resize of local files
		query := r.URL.Query()
		imageWidth := query.Get("image_width")
		if imageWidth != "" {
			size, err := parseSize(imageWidth)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, err.Error())
				return
			}

			if v := query.Get("clear"); v == s.ClearCode {
				if err := clearCache(s, r.URL.Path, size); err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Fprintf(w, err.Error())
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			url, err := cacheAndServe(s, r.URL.Path, size)
			if os.IsNotExist(err) {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprintf(w, "Not found.")
				fmt.Println(err)
				return
			} else if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "An error occured.")
				fmt.Println(err)
				return
			}
			http.Redirect(w, r, url, http.StatusFound)
		}

		w.WriteHeader(http.StatusNotFound)
	}
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

	// Let's resize the image, serve it, and cache it
	reader, err := os.Open(s.GetAssetPath(filename))
	if err != nil {
		return "", err
	}
	defer reader.Close()

	buf, err := ioutil.ReadAll(reader)
	if err != nil {
		return "", err
	}

	decoder, err := lilliput.NewDecoder(buf)
	if err != nil {
		return "", err
	}
	defer decoder.Close()

	header, err := decoder.Header()
	if err != nil {
		return "", err
	}

	var ratio float64
	if header.Width() <= 0 {
		ratio = 1
	} else {
		ratio = float64(header.Height()) / float64(header.Width())
	}

	ops := lilliput.NewImageOps(MaxSize)
	defer ops.Close()

	img := make([]byte, 50*1024*1024)
	opts := &lilliput.ImageOptions{
		FileType:     "." + strings.ToLower(decoder.Description()),
		Width:        int(size),
		Height:       int(float64(size) * ratio),
		ResizeMethod: lilliput.ImageOpsResize,
	}
	img, err = ops.Transform(decoder, opts, img)
	if err != nil {
		return "", err
	}

	err = s3util.UploadS3(s, bytes.NewReader(img), mime.TypeByExtension(filepath.Ext(filename)), s3.BucketCannedACLPublicRead, key)
	return s.MkURL(key), err
}