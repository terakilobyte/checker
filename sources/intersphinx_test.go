package sources

import (
	"bytes"
	"compress/zlib"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestNoContents(t *testing.T) {

	logrus.SetOutput(ioutil.Discard)

	header := ""
	r := ioutil.NopCloser(strings.NewReader(header))

	Client = &MockClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       r,
			}, nil
		},
	}
	resp := Intersphinx("test")
	if resp != nil {
		t.Errorf("Expected nil, got %v", resp)
	}

}

func TestInvalidHeader(t *testing.T) {

	logrus.SetOutput(ioutil.Discard)
	header := `# Sphinx inventory version 2
# Project: golang
# Version:
`
	r := ioutil.NopCloser(strings.NewReader(header))

	Client = &MockClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       r,
			}, nil
		},
	}
	resp := Intersphinx("test")
	if resp != nil {
		t.Errorf("Expected nil, got %v", resp)
	}

}
func TestHeaderNoContent(t *testing.T) {

	header := `# Sphinx inventory version 2
# Project: golang
# Version:
# The remainder of this file is compressed using zlib.
`

	r := ioutil.NopCloser(strings.NewReader(header))

	Client = &MockClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       r,
			}, nil
		},
	}
	resp := Intersphinx("test")
	if resp != nil {
		t.Errorf("Expected nil, got %v", resp)
	}

}

func TestInvalidContent(t *testing.T) {
	logrus.SetOutput(ioutil.Discard)

	header := `# Sphinx inventory version 2
# Project: golang
# Version:
# The remainder of this file is compressed using zlib.
`
	zText := []byte(`whats-new std:doc -1 whats-new/ What's New
compatibility std:doc -1 compatibility/ Compatibility
fundamentals std:doc -1 fundamentals/ Fundamentals
usage-examples std:doc -1 usage-examples/ Usage Examples`)

	r := ioutil.NopCloser(strings.NewReader(header + string(zText)))

	Client = &MockClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       r,
			}, nil
		},
	}
	resp := Intersphinx("test")

	if resp != nil {
		t.Errorf("Expected nil, got %v", resp)
	}

}

func TestSomeContent(t *testing.T) {
	logrus.SetOutput(ioutil.Discard)

	header := `# Sphinx inventory version 2
# Project: golang
# Version:
# The remainder of this file is compressed using zlib.
`
	zText := []byte(`whats-new std:doc -1 whats-new/ What's New
compatibility std:doc -1 compatibility/ Compatibility
fundamentals std:doc -1 fundamentals/ Fundamentals
usage-examples std:doc -1 usage-examples/ Usage Examples`)

	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	w.Write(zText)
	w.Close()

	r := ioutil.NopCloser(strings.NewReader(header + b.String()))

	Client = &MockClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       r,
			}, nil
		},
	}
	resp := Intersphinx("https://docs.mongodb.com/drivers/go/current/objects.inv")

	if len(resp) != 4 {
		t.Errorf("Expected 4 entries, got %v", len(resp))
	}

	expected := RefMap{
		"whats-new":      Ref{Target: "https://docs.mongodb.com/drivers/go/current/whats-new/", Type: "intersphinx"},
		"compatibility":  Ref{Target: "https://docs.mongodb.com/drivers/go/current/compatibility/", Type: "intersphinx"},
		"fundamentals":   Ref{Target: "https://docs.mongodb.com/drivers/go/current/fundamentals/", Type: "intersphinx"},
		"usage-examples": Ref{Target: "https://docs.mongodb.com/drivers/go/current/usage-examples/", Type: "intersphinx"},
	}

	for k, v := range resp {
		if v != expected[k] {
			t.Errorf("Expected %v, got %v", expected[k], v)
		}
	}

}
