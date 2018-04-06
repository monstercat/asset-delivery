package main

import (
	"context"
	"cloud.google.com/go/storage"
	"errors"
	"fmt"
	"github.com/discordapp/lilliput"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"io/ioutil"
	"path/filepath"
	"net/http"
	"strconv"
	"strings"
)

const (
	ENDPOINT = "https://storage.googleapis.com/mcat-01-bucket-01/"
	ASSETS_PATH = "assets"
	CACHE_PATH = "asset-delivery-cache"
)

func main () {
	ctx := context.Background()
	clientOpts := option.WithCredentialsFile("./key.json")
	client, err := storage.NewClient(ctx, clientOpts)
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
	bucket := client.Bucket("mcat-01-bucket-01")

	http.HandleFunc("/", func (w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, "GET methods only.")
			return
		}

		objPath := assetPath(r.URL.Path)
		query := r.URL.Query()
		image_width := query.Get("image_width")
		if image_width != "" {
			objPath, err = makeImage(bucket, r.URL.Path, image_width)
			if errCode(err) == 404 {
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
		}

		obj := bucket.Object(objPath)
		acl := obj.ACL()
		acls, err := acl.List(ctx)
		if errCode(err) == 404 {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, "Not found.")
			return
		} else if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "An error occured.")
			fmt.Println(err)
			return
		}

		// All assets should be publicly available
		if !isPublic(acls) {
			acl.Set(ctx, storage.AllUsers, storage.RoleReader)
		}

		http.Redirect(w, r, ENDPOINT + objPath, http.StatusFound)
	})

    err = http.ListenAndServe(":1337", nil)
    if err != nil {
    	panic(err)
    }
}

func isPublic (acls []storage.ACLRule) bool {
	value := false
	for _, rule := range acls {
		if rule.Entity == storage.AllUsers && rule.Role == storage.RoleReader {
			value = true
		}
	}
	return value
}

func errCode (err error) int {
	if err, ok := err.(*googleapi.Error); !ok {
		return -1
	} else {
		return err.Code
	}
}

func assetPath (str string) string {
	return filepath.Join(ASSETS_PATH, str)
}

func imagePath (str string, size string) string {
	filename := size + "_" + filepath.Base(str)
	return filepath.Join(CACHE_PATH, filepath.Dir(str), filename)
}

func makeImage (bucket *storage.BucketHandle, str string, size string) (string, error) {
	cpath := imagePath(str, size)
	usize, err := strconv.ParseUint(size, 10, 32)
	if err != nil {
		return cpath, err
	}
	if usize > 4096 {
		return cpath, errors.New("\"image_width\" cannot exceed 4096.")
	}

	ctx := context.Background()
	cobj := bucket.Object(cpath)
	reader, err := cobj.NewReader(ctx)
	if err != nil {
		if err != storage.ErrObjectNotExist {
			return cpath, err
		}
	} else {
		reader.Close()
		return cpath, nil
	}

	apath := assetPath(str)
	aobj := bucket.Object(apath)
	reader, err = aobj.NewReader(ctx)
	if err != nil {
		return cpath, err
	}
	defer reader.Close()

	buf, err := ioutil.ReadAll(reader)
	if err != nil {
		return cpath, err
	}

	decoder, err := lilliput.NewDecoder(buf)
	if err != nil {
		return cpath, err
	}
	defer decoder.Close()

	header, err := decoder.Header()
	if err != nil {
		return cpath, err
	}

	ratio := float64(header.Height()) / float64(header.Width())
	ops := lilliput.NewImageOps(4096)
	defer ops.Close()

	img := make([]byte, 50*1024*1024)
	opts := &lilliput.ImageOptions{
		FileType:     "." + strings.ToLower(decoder.Description()),
		Width:        int(usize),
		Height:       int(float64(usize) * ratio),
		ResizeMethod: lilliput.ImageOpsResize,
	}
	img, err = ops.Transform(decoder, opts, img)
	if err != nil {
		return cpath, err
	}

	writer := cobj.NewWriter(ctx)
	if err != nil {
		return cpath, err
	}
	defer writer.Close()

	_, err = writer.Write(img)
	if err != nil {
		return cpath, err
	}

	return cpath, nil
}