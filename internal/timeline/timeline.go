// Package timeline owns where a Scenario's Timeline is kept: the folder a run's
// Timelines collect in, the file one Timeline is named after, and the JSON Lines
// format those files carry.
//
// It imports internal/scenario and the standard library alone: a Timeline
// reaches it as immutable values, so the published line format can never become
// the Kubernetes client's shape, per
// docs/adr/2026-10-05-timeline-from-the-watch-stream.md.
package timeline

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	// folderName is the one folder under the working directory a run's Timelines
	// collect in, and fileSuffix marks a filed Timeline as the JSON Lines it is.
	folderName = "timelines"
	fileSuffix = ".jsonl"

	// folderPermissions and filePermissions make a created folder and a created
	// file readable and writable by the invoking user alone: a Timeline outlives
	// its run, in a working directory other users may reach
	// (docs/system-design.md § Threat Model).
	folderPermissions = 0o700
	filePermissions   = 0o600
)

// The record field every line carries, so a reader of a filed Timeline tells
// the header from the entries without counting lines.
const (
	headerRecord = "timeline"
	entryRecord  = "entry"
)

// Folder is where one run's Timelines collect: the folder under the root the
// run resolved. It is created when that run opens its first Timeline in it, so a
// run that files nothing leaves nothing behind.
type Folder struct {
	path string
}

// NewFolder is the Timeline folder under root.
func NewFolder(root string) Folder {
	return Folder{path: filepath.Join(root, folderName)}
}

// Path is where this folder's Timelines are filed.
func (f Folder) Path() string {
	return f.path
}

// Open is the file the Timeline this header describes is filed in, named for the
// Simulated Cluster that Timeline is of. A folder or a file it cannot open is
// the lab's own failure and not the Scenario's: the File returned reports that
// error and discards every entry, so the Scenario still runs and its outcome
// still stands, per docs/adr/2026-10-05-timeline-from-the-watch-stream.md.
func (f Folder) Open(header Header) *File {
	name, err := fileNameOf(header.Cluster)
	if err != nil {
		return &File{err: err}
	}

	path := filepath.Join(f.path, name)
	if err := os.MkdirAll(f.path, folderPermissions); err != nil {
		return &File{path: path, err: fmt.Errorf("create the Timeline folder %s: %w", f.path, err)}
	}

	opened, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePermissions)
	if err != nil {
		return &File{path: path, err: fmt.Errorf("open the Timeline file %s: %w", path, err)}
	}

	lines := json.NewEncoder(opened)
	// A control plane's message is text, not HTML: escaping its angle brackets
	// would file a line that no longer reads as what the control plane said.
	lines.SetEscapeHTML(false)

	filed := &File{path: path, to: opened, lines: lines}
	filed.write(headerLine{
		Record: headerRecord, Scenario: header.Scenario, Cluster: header.Cluster, Kubernetes: header.Kubernetes,
	})

	return filed
}

// fileNameOf is the file one Simulated Cluster's Timeline is filed as. The name
// is joined to the folder as one path element, so a name holding a separator or
// a parent reference is refused rather than filed somewhere else
// (docs/system-design.md § Threat Model).
func fileNameOf(cluster string) (string, error) {
	if cluster == "" || cluster == "." || cluster == ".." ||
		cluster != filepath.Base(cluster) || strings.ContainsAny(cluster, `/\`) {
		return "", fmt.Errorf("%q names no Simulated Cluster a Timeline can be filed for: a Timeline file is one element of the %s folder", cluster, folderName)
	}

	return cluster + fileSuffix, nil
}

// Header is the first line of a filed Timeline: the Scenario it records, the
// Simulated Cluster that Scenario ran on, and the Kubernetes version that
// cluster mirrors.
type Header struct {
	Scenario   string
	Cluster    string
	Kubernetes string
}

// File is one Scenario's filed Timeline. It writes its header when it is
// opened and one JSON object per entry as that entry arrives, which is what
// leaves a usable file behind when a Scenario fails part-way through its Steps.
//
// It keeps its first write error and discards every entry after it, the way a
// run's report keeps its first write error: a Timeline the lab could not write
// is reported once, by the run, rather than checked at every entry.
type File struct {
	path  string
	to    io.Closer
	lines *json.Encoder
	err   error
}

// Path is where this Timeline was filed, empty for a Simulated Cluster name no
// Timeline could be filed under.
func (f *File) Path() string {
	return f.path
}

// Write files one entry of the Timeline. It is the sink a recording delivers
// its entries to.
func (f *File) Write(entry scenario.Entry) {
	f.write(entryLine{
		Record: entryRecord, Elapsed: entry.Elapsed.String(), Change: string(entry.Change),
		Kind: string(entry.Kind), Name: entry.Name, Reason: entry.Reason, Detail: entry.Detail,
	})
}

// write files one line unless an earlier line already failed. The encoder
// terminates every line with a newline, which is what makes the file JSON Lines.
func (f *File) write(line any) {
	if f.err != nil {
		return
	}

	if err := f.lines.Encode(line); err != nil {
		f.err = fmt.Errorf("write the Timeline %s: %w", f.path, err)
	}
}

// Close closes this file, keeping a close error the way a write error is kept,
// so what went wrong is reported once, through Err. Closing a Timeline that was
// never opened closes nothing.
func (f *File) Close() {
	if f.to == nil {
		return
	}

	closed := f.to.Close()
	f.to, f.lines = nil, nil
	if closed != nil && f.err == nil {
		f.err = fmt.Errorf("close the Timeline %s: %w", f.path, closed)
	}
}

// Err is what prevented this Timeline from being filed, which the run names.
func (f *File) Err() error {
	return f.err
}

// headerLine and entryLine are the published JSON Lines form of a filed
// Timeline. They are this package's own types rather than the Timeline values
// themselves, so the format a later reader depends on is declared here and
// changes only here.
type headerLine struct {
	Record     string `json:"record"`
	Scenario   string `json:"scenario"`
	Cluster    string `json:"cluster"`
	Kubernetes string `json:"kubernetes"`
}

type entryLine struct {
	Record  string `json:"record"`
	Elapsed string `json:"elapsed"`
	Change  string `json:"change"`
	Kind    string `json:"kind,omitempty"`
	Name    string `json:"name,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Detail  string `json:"detail,omitempty"`
}
