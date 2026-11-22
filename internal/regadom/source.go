package regadom

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Source is what one upload or one file on the system gave: the database, and from a backup the
// firmware version and the location the CCU was set to.
type Source struct {
	Kind      string // "sbk" or "regadom"
	DB        *DB
	Firmware  string // the backup's firmware_version, e.g. 3.89.8.20260719
	Latitude  string // time.conf of the backup; the database's own gateway-config when that is empty
	Longitude string
	Timezone  string

	keep bool  // read keeps root
	root *node // the database's elements
}

// Errors of Read.
var (
	ErrNoArchive  = errors.New("the backup has no usr_local.tar.gz")
	ErrNoDatabase = errors.New("the backup has no ReGa database (homematic.regadom)")
)

// Read takes a CCU backup (.sbk) or a bare homematic.regadom and parses it. A backup is a tar
// holding usr_local.tar.gz, which holds usr/local/etc/config/homematic.regadom and, when the user
// ever set the location, time.conf. Nothing is written to disk and the archive is not restored.
func Read(r io.Reader) (*Source, error) {
	return read(r, false)
}

// read is Read; with tree the source keeps the database's elements as well (the anonymiser
// rewrites them).
func read(r io.Reader, tree bool) (*Source, error) {
	br := bufio.NewReaderSize(r, 4096)
	head, _ := br.Peek(512)
	if looksLikeXML(head) {
		db, root, err := parse(br)
		if err != nil {
			return nil, err
		}
		src := &Source{Kind: "regadom", DB: db, Latitude: db.Latitude, Longitude: db.Longitude}
		if tree {
			src.root = root
		}
		return src, nil
	}
	src := &Source{Kind: "sbk", keep: tree}
	outer := tar.NewReader(br)
	found := false
	for {
		h, err := outer.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("backup: %w", err)
		}
		switch strings.TrimPrefix(h.Name, "./") {
		case "firmware_version":
			src.Firmware = keyValues(readSmall(outer))["VERSION"]
		case "usr_local.tar.gz":
			found = true
			if err := src.readInner(outer); err != nil {
				return nil, err
			}
		}
	}
	if !found {
		return nil, ErrNoArchive
	}
	if src.DB == nil {
		return nil, ErrNoDatabase
	}
	if src.Latitude == "" && src.Longitude == "" {
		src.Latitude, src.Longitude = src.DB.Latitude, src.DB.Longitude
	}
	return src, nil
}

func (src *Source) readInner(r io.Reader) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("usr_local.tar.gz: %w", err)
	}
	inner := tar.NewReader(gz)
	for {
		h, err := inner.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("usr_local.tar.gz: %w", err)
		}
		switch {
		case strings.HasSuffix(h.Name, "etc/config/homematic.regadom"):
			db, root, err := parse(inner)
			if err != nil {
				return err
			}
			src.DB = db
			if src.keep {
				src.root = root
			}
		case strings.HasSuffix(h.Name, "etc/config/time.conf"):
			kv := keyValues(readSmall(inner))
			src.Latitude, src.Longitude, src.Timezone = kv["LATITUDE"], kv["LONGITUDE"], kv["TIMEZONE"]
		}
	}
}

// looksLikeXML tells a bare database from a tar: the first thing that is not white space or a
// byte order mark is a '<'.
func looksLikeXML(head []byte) bool {
	head = bytes.TrimPrefix(head, []byte("\xef\xbb\xbf"))
	head = bytes.TrimLeft(head, " \t\r\n")
	return len(head) > 0 && head[0] == '<'
}

func readSmall(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 64<<10))
	return string(b)
}

// keyValues reads KEY=VALUE lines (time.conf, firmware_version); quotes around a value go.
func keyValues(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return out
}
