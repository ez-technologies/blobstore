package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ez-technologies/blobstore"
)

func TestUploadListDownloadDelete(t *testing.T) {
	store := blobstore.NewMemory()
	srv := httptest.NewServer(newHandler(store))
	defer srv.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "notes.txt")
	_, _ = fw.Write([]byte("hello"))
	_ = mw.Close()
	res, err := http.Post(srv.URL+"/files", mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	var f file
	_ = json.NewDecoder(res.Body).Decode(&f)
	res.Body.Close()
	if res.StatusCode != http.StatusCreated || f.Name != "notes.txt" || f.Size != 5 {
		t.Fatalf("upload: status %d, %+v", res.StatusCode, f)
	}
	if keys := store.Keys(); len(keys) != 1 || keys[0] != "files/"+f.ID {
		t.Fatalf("stored under %v, want the opaque key files/%s", keys, f.ID)
	}

	res, _ = http.Get(srv.URL + "/files")
	var list []file
	_ = json.NewDecoder(res.Body).Decode(&list)
	res.Body.Close()
	if len(list) != 1 || list[0].ID != f.ID {
		t.Fatalf("list: %+v", list)
	}

	res, _ = http.Get(srv.URL + "/files/" + f.ID)
	got, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(got) != "hello" {
		t.Fatalf("download: %q", got)
	}

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/files/"+f.ID, nil)
	res, _ = http.DefaultClient.Do(req)
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: status %d", res.StatusCode)
	}
	res, _ = http.Get(srv.URL + "/files/" + f.ID)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("download after delete: status %d", res.StatusCode)
	}
}
