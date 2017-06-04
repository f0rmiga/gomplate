package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/blang/vfs"
	"github.com/hairyhenderson/gomplate/vault"
)

// logFatal is defined so log.Fatal calls can be overridden for testing
var logFatalf = log.Fatalf

func init() {
	// Add some types we want to be able to handle which can be missing by default
	err := mime.AddExtensionType(".json", "application/json")
	if err != nil {
		log.Fatal(err)
	}
	err = mime.AddExtensionType(".yml", "application/yaml")
	if err != nil {
		log.Fatal(err)
	}
	err = mime.AddExtensionType(".yaml", "application/yaml")
	if err != nil {
		log.Fatal(err)
	}
	err = mime.AddExtensionType(".csv", "text/csv")
	if err != nil {
		log.Fatal(err)
	}

	sourceReaders = make(map[string]func(*Source, ...string) ([]byte, error))

	// Register our source-reader functions
	addSourceReader("http", readHTTP)
	addSourceReader("https", readHTTP)
	addSourceReader("file", readFile)
	addSourceReader("vault", readVault)
}

var sourceReaders map[string]func(*Source, ...string) ([]byte, error)

// addSourceReader -
func addSourceReader(scheme string, readFunc func(*Source, ...string) ([]byte, error)) {
	sourceReaders[scheme] = readFunc
}

// Data -
type Data struct {
	Sources map[string]*Source
	cache   map[string][]byte
}

// NewData - constructor for Data
func NewData(datasourceArgs []string, headerArgs []string) *Data {
	sources := make(map[string]*Source)
	headers := parseHeaderArgs(headerArgs)
	for _, v := range datasourceArgs {
		s, err := ParseSource(v)
		if err != nil {
			log.Fatalf("error parsing datasource %v", err)
			return nil
		}
		s.Header = headers[s.Alias]
		sources[s.Alias] = s
	}
	return &Data{
		Sources: sources,
	}
}

// Source - a data source
type Source struct {
	Alias  string
	URL    *url.URL
	Ext    string
	Type   string
	Params map[string]string
	FS     vfs.Filesystem // used for file: URLs, nil otherwise
	HC     *http.Client   // used for http[s]: URLs, nil otherwise
	VC     *vault.Client  //used for vault: URLs, nil otherwise
	Header http.Header    // used for http[s]: URLs, nil otherwise
}

// NewSource - builds a &Source
func NewSource(alias string, URL *url.URL) (s *Source) {
	ext := filepath.Ext(URL.Path)

	s = &Source{
		Alias: alias,
		URL:   URL,
		Ext:   ext,
	}

	if ext != "" {
		mediatype := mime.TypeByExtension(ext)
		t, params, err := mime.ParseMediaType(mediatype)
		if err != nil {
			log.Fatal(err)
		}
		s.Type = t
		s.Params = params
	}
	return
}

// String is the method to format the flag's value, part of the flag.Value interface.
// The String method's output will be used in diagnostics.
func (s *Source) String() string {
	return fmt.Sprintf("%s=%s (%s)", s.Alias, s.URL.String(), s.Type)
}

// ParseSource -
func ParseSource(value string) (*Source, error) {
	var (
		alias  string
		srcURL *url.URL
	)
	parts := strings.SplitN(value, "=", 2)
	if len(parts) == 1 {
		f := parts[0]
		alias = strings.SplitN(value, ".", 2)[0]
		if path.Base(f) != f {
			err := fmt.Errorf("Invalid datasource (%s). Must provide an alias with files not in working directory", value)
			return nil, err
		}
		srcURL = absURL(f)
	} else if len(parts) == 2 {
		alias = parts[0]
		var err error
		srcURL, err = url.Parse(parts[1])
		if err != nil {
			return nil, err
		}

		if !srcURL.IsAbs() {
			srcURL = absURL(parts[1])
		}
	}

	s := NewSource(alias, srcURL)
	return s, nil
}

func absURL(value string) *url.URL {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Can't get working directory: %s", err)
	}
	baseURL := &url.URL{
		Scheme: "file",
		Path:   cwd + "/",
	}
	relURL := &url.URL{
		Path: value,
	}
	return baseURL.ResolveReference(relURL)
}

// DatasourceExists -
func (d *Data) DatasourceExists(alias string) bool {
	_, ok := d.Sources[alias]
	return ok
}

// Datasource -
func (d *Data) Datasource(alias string, args ...string) interface{} {
	source, ok := d.Sources[alias]
	if !ok {
		log.Fatalf("Undefined datasource '%s'", alias)
	}
	b, err := d.ReadSource(source.FS, source, args...)
	if err != nil {
		log.Fatalf("Couldn't read datasource '%s': %s", alias, err)
	}
	s := string(b)
	if source.Type == "application/json" {
		ty := &TypeConv{}
		return ty.JSON(s)
	}
	if source.Type == "application/yaml" {
		ty := &TypeConv{}
		return ty.YAML(s)
	}
	if source.Type == "text/csv" {
		ty := &TypeConv{}
		return ty.CSV(s)
	}
	log.Fatalf("Datasources of type %s not yet supported", source.Type)
	return nil
}

// Include -
func (d *Data) include(alias string, args ...string) interface{} {
	source, ok := d.Sources[alias]
	if !ok {
		log.Fatalf("Undefined datasource '%s'", alias)
	}
	b, err := d.ReadSource(source.FS, source, args...)
	if err != nil {
		log.Fatalf("Couldn't read datasource '%s': %s", alias, err)
	}
	return string(b)
}

// ReadSource -
func (d *Data) ReadSource(fs vfs.Filesystem, source *Source, args ...string) ([]byte, error) {
	if d.cache == nil {
		d.cache = make(map[string][]byte)
	}
	cacheKey := source.Alias
	for _, v := range args {
		cacheKey += v
	}
	cached, ok := d.cache[cacheKey]
	if ok {
		return cached, nil
	}
	if r, ok := sourceReaders[source.URL.Scheme]; ok {
		data, err := r(source, args...)
		if err != nil {
			return nil, err
		}
		d.cache[cacheKey] = data
		return data, nil
	}

	log.Fatalf("Datasources with scheme %s not yet supported", source.URL.Scheme)
	return nil, nil
}

func readFile(source *Source, args ...string) ([]byte, error) {
	if source.FS == nil {
		source.FS = vfs.OS()
	}

	// make sure we can access the file
	_, err := source.FS.Stat(source.URL.Path)
	if err != nil {
		log.Fatalf("Can't stat %s: %#v", source.URL.Path, err)
		return nil, err
	}

	f, err := source.FS.OpenFile(source.URL.Path, os.O_RDONLY, 0)
	if err != nil {
		log.Fatalf("Can't open %s: %#v", source.URL.Path, err)
		return nil, err
	}

	b, err := ioutil.ReadAll(f)
	if err != nil {
		log.Fatalf("Can't read %s: %#v", source.URL.Path, err)
		return nil, err
	}
	return b, nil
}

func readHTTP(source *Source, args ...string) ([]byte, error) {
	if source.HC == nil {
		source.HC = &http.Client{Timeout: time.Second * 5}
	}
	req, err := http.NewRequest("GET", source.URL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header = source.Header
	res, err := source.HC.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	err = res.Body.Close()
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		err := fmt.Errorf("Unexpected HTTP status %d on GET from %s: %s", res.StatusCode, source.URL, string(body))
		return nil, err
	}
	ctypeHdr := res.Header.Get("Content-Type")
	if ctypeHdr != "" {
		mediatype, params, e := mime.ParseMediaType(ctypeHdr)
		if e != nil {
			return nil, e
		}
		source.Type = mediatype
		source.Params = params
	}
	return body, nil
}

func readVault(source *Source, args ...string) ([]byte, error) {
	if source.VC == nil {
		source.VC = vault.NewClient()
		err := source.VC.Login()
		addCleanupHook(source.VC.RevokeToken)
		if err != nil {
			return nil, err
		}
	}

	p := source.URL.Path
	if len(args) == 1 {
		p = p + "/" + args[0]
	}

	data, err := source.VC.Read(p)
	if err != nil {
		return nil, err
	}
	source.Type = "application/json"

	return data, nil
}

func parseHeaderArgs(headerArgs []string) map[string]http.Header {
	headers := make(map[string]http.Header)
	for _, v := range headerArgs {
		ds, name, value := splitHeaderArg(v)
		if _, ok := headers[ds]; !ok {
			headers[ds] = make(http.Header)
		}
		headers[ds][name] = append(headers[ds][name], strings.TrimSpace(value))
	}
	return headers
}

func splitHeaderArg(arg string) (datasourceAlias, name, value string) {
	parts := strings.SplitN(arg, "=", 2)
	if len(parts) != 2 {
		logFatalf("Invalid datasource-header option '%s'", arg)
		return "", "", ""
	}
	datasourceAlias = parts[0]
	name, value = splitHeader(parts[1])
	return datasourceAlias, name, value
}

func splitHeader(header string) (name, value string) {
	parts := strings.SplitN(header, ":", 2)
	if len(parts) != 2 {
		logFatalf("Invalid HTTP Header format '%s'", header)
		return "", ""
	}
	name = http.CanonicalHeaderKey(parts[0])
	value = parts[1]
	return name, value
}
