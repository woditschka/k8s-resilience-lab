// Package timeline owns where a Scenario's Timeline is kept and the JSON Lines
// format it is filed in. It imports internal/scenario and the standard library
// alone, so the published format can never become the Kubernetes client's
// shape.
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
	folderName = "timelines"
	fileSuffix = ".jsonl"

	// A Timeline outlives its run in a working directory other users may reach,
	// so only the invoking user may read or write it.
	folderPermissions = 0o700
	filePermissions   = 0o600
)

// The record field lets a reader tell the header from the entries without
// counting lines.
const (
	headerRecord = "timeline"
	entryRecord  = "entry"
)

// Folder is created when the run opens its first Timeline, so a run that files
// nothing leaves nothing behind.
type Folder struct {
	path string
}

func NewFolder(root string) Folder {
	return Folder{path: filepath.Join(root, folderName)}
}

func (f Folder) Path() string {
	return f.path
}

// Open treats a folder or file it cannot create as the lab's own failure, not
// the Scenario's: the File returned reports the error and discards every entry,
// so the Scenario still runs.
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

// The name is joined as one path element, so a name holding a separator or a
// parent reference is refused rather than filed somewhere else.
func fileNameOf(cluster string) (string, error) {
	if cluster == "" || cluster == "." || cluster == ".." ||
		cluster != filepath.Base(cluster) || strings.ContainsAny(cluster, `/\`) {
		return "", fmt.Errorf("%q names no Simulated Cluster a Timeline can be filed for: a Timeline file is one element of the %s folder", cluster, folderName)
	}

	return cluster + fileSuffix, nil
}

type Header struct {
	Scenario   string
	Cluster    string
	Kubernetes string
}

// File writes each entry as it arrives, which leaves a usable file behind when
// a Scenario fails part-way. It keeps its first write error and discards every
// entry after it, so a lost Timeline is reported once, by the run.
type File struct {
	path  string
	to    io.Closer
	lines *json.Encoder
	err   error
}

// Path is empty where the Simulated Cluster name could not be filed under.
func (f *File) Path() string {
	return f.path
}

func (f *File) Write(entry scenario.Entry) {
	f.write(entryLine{
		Record: entryRecord, Elapsed: entry.Elapsed.String(), Change: string(entry.Change),
		Kind: string(entry.Kind), Name: entry.Name, Reason: entry.Reason, Detail: entry.Detail,
	})
}

// The encoder terminates every line with a newline, which is what makes the
// file JSON Lines.
func (f *File) write(line any) {
	if f.err != nil {
		return
	}

	if err := f.lines.Encode(line); err != nil {
		f.err = fmt.Errorf("write the Timeline %s: %w", f.path, err)
	}
}

// Close keeps a close error like a write error. Closing a Timeline that was
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

func (f *File) Err() error {
	return f.err
}

// These are the package's own types rather than the Timeline values, so the
// format a later reader depends on changes only here.
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
