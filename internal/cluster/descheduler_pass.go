package cluster

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// No run input reaches the repository, so a run can never start an image that
// is not the Descheduler's.
const deschedulerImageRepository = "registry.k8s.io/descheduler/descheduler"

// The image holds the binary alone and declares no entrypoint, so the command
// names it.
const (
	deschedulerBinary         = "/bin/descheduler"
	deschedulerDirectory      = "etc/descheduler/"
	deschedulerPolicyPath     = "/" + deschedulerDirectory + "policy.json"
	deschedulerKubeconfigPath = "/" + deschedulerDirectory + "kubeconfig"

	// A zero interval makes the Descheduler run one pass and exit, which is
	// how the lab learns the pass ended.
	policyArgument     = "--policy-config-file=" + deschedulerPolicyPath
	kubeconfigArgument = "--kubeconfig=" + deschedulerKubeconfigPath
	onePassArgument    = "--descheduling-interval=0s"
)

// The command is constants alone, so no run input reaches the pass container's
// arguments.
var deschedulerPassArguments = []string{
	deschedulerBinary,
	policyArgument,
	kubeconfigArgument,
	onePassArgument,
}

// The pass shares its cluster's network namespace, so the API server answers
// on loopback. The API server is unauthenticated, so the kubeconfig carries no
// user and no credential.
const deschedulerKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: simulated-cluster
  cluster:
    server: http://` + loopbackAddress + `:` + apiServerContainerPort + `
contexts:
- name: simulated-cluster
  context:
    cluster: simulated-cluster
current-context: simulated-cluster
`

// The suffix keeps the pass beside its cluster in the engine's own listing,
// and under the lab's prefix a reader removes leftovers by.
const deschedulerPassNameSuffix = "-descheduler"

// The image runs as an unprivileged user, so the files it reads are readable
// by every user and writable by none.
const (
	readableDirectory = 0o755
	readableFile      = 0o444
)

// A failed pass is explained by its last lines, and a pass that never ends
// would otherwise hand the lab its whole log.
const (
	lastLogLines       = 20
	logBytesReadAtMost = 64 << 10
)

// The engine prefixes every frame of a log read without a terminal with a
// header naming the stream and the frame's length.
const (
	logFrameHeaderLength = 8
	logFrameLengthAt     = 4
)

func deschedulerImage(version DeschedulerVersion) string {
	return deschedulerImageRepository + ":" + version.String()
}

func deschedulerPassNameFor(clusterName string) string {
	return clusterName + deschedulerPassNameSuffix
}

// RunDeschedulerPass returns once the pass's container exited zero and was
// removed, reporting whether the pass evicted a Pod. The container is removed
// on every path, under a context its caller cannot cancel, so a Scenario
// stopped mid-pass leaves nothing behind.
//
// The eviction is read from the Pods listed before the pass starts and after it
// exits rather than from its log, whose wording is the Descheduler's own.
func (c *SimulatedCluster) RunDeschedulerPass(ctx context.Context, policy scenario.DeschedulerPolicy, version DeschedulerVersion) (evicted bool, err error) {
	archive, err := deschedulerPassArchive(policy)
	if err != nil {
		return false, err
	}

	image := deschedulerImage(version)
	if err := (Engine{endpoint: c.container.endpoint}).pullWhenAbsent(ctx, image); err != nil {
		return false, fmt.Errorf("obtain the Descheduler %s: %w", version, err)
	}

	pass, err := c.container.createDeschedulerPass(ctx, image)
	if err != nil {
		return false, err
	}
	c.pass.Store(pass)
	defer func() {
		err = errors.Join(err, c.removeHeldDeschedulerPass(context.WithoutCancel(ctx)))
	}()

	before, err := c.everyDeploymentsPods(ctx)
	if err != nil {
		return false, err
	}
	if err := pass.run(ctx, archive); err != nil {
		return false, err
	}
	after, err := c.everyDeploymentsPods(ctx)
	if err != nil {
		return false, err
	}

	return anyEvicted(before, after), nil
}

func (c *SimulatedCluster) everyDeploymentsPods(ctx context.Context) ([]corev1.Pod, error) {
	listed, err := c.client.CoreV1().Pods(clusterNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: everyDeploymentsPods(),
	})
	if err != nil {
		return nil, fmt.Errorf("list the Pods of every Deployment of %s around a Descheduler pass: %w", c.Name(), err)
	}

	return listed.Items, nil
}

// A removal that fails leaves the pass held, so Close tries it once more.
func (c *SimulatedCluster) removeHeldDeschedulerPass(ctx context.Context) error {
	held := c.pass.Swap(nil)
	if held == nil {
		return nil
	}
	if err := held.remove(ctx); err != nil {
		c.pass.CompareAndSwap(nil, held)

		return err
	}

	return nil
}

func deschedulerPassArchive(policy scenario.DeschedulerPolicy) ([]byte, error) {
	encoded, err := toDeschedulerPolicy(policy)
	if err != nil {
		return nil, err
	}

	var archive bytes.Buffer
	written := tar.NewWriter(&archive)
	if err := written.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: deschedulerDirectory, Mode: readableDirectory}); err != nil {
		return nil, fmt.Errorf("archive the Descheduler's directory: %w", err)
	}
	for _, file := range []struct {
		path    string
		content []byte
	}{
		{path: deschedulerKubeconfigPath, content: []byte(deschedulerKubeconfig)},
		{path: deschedulerPolicyPath, content: encoded},
	} {
		header := &tar.Header{Typeflag: tar.TypeReg, Name: strings.TrimPrefix(file.path, "/"), Mode: readableFile, Size: int64(len(file.content))}
		if err := written.WriteHeader(header); err != nil {
			return nil, fmt.Errorf("archive %s for the Descheduler: %w", file.path, err)
		}
		if _, err := written.Write(file.content); err != nil {
			return nil, fmt.Errorf("archive %s for the Descheduler: %w", file.path, err)
		}
	}
	if err := written.Close(); err != nil {
		return nil, fmt.Errorf("archive the Descheduler's files: %w", err)
	}

	return archive.Bytes(), nil
}

// deschedulerPass declares no call taking an identifier from a caller, as
// ClusterContainer does not.
type deschedulerPass struct {
	endpoint   Endpoint
	identifier string
	name       string
}

// The pass joins the cluster's network namespace alone: it publishes no port,
// mounts nothing, drops every capability and gains no privilege.
func (c *ClusterContainer) createDeschedulerPass(ctx context.Context, image string) (*deschedulerPass, error) {
	name := deschedulerPassNameFor(c.name)
	requested, err := json.Marshal(map[string]any{
		"Image": image,
		"Cmd":   deschedulerPassArguments,
		"HostConfig": map[string]any{
			"NetworkMode": "container:" + c.identifier,
			"CapDrop":     []string{"ALL"},
			"SecurityOpt": []string{"no-new-privileges"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("compose the request creating the Descheduler pass %s: %w", name, err)
	}

	var created struct {
		ID string `json:"Id"`
	}
	query := url.Values{"name": {name}}
	if err := engineRequest(ctx, c.endpoint, http.MethodPost, "/containers/create?"+query.Encode(), bytes.NewReader(requested), &created); err != nil {
		return nil, fmt.Errorf("create the Descheduler pass %s: %w", name, err)
	}
	if created.ID == "" {
		return nil, fmt.Errorf("create the Descheduler pass %s: the engine named no container", name)
	}

	return &deschedulerPass{endpoint: c.endpoint, identifier: created.ID, name: name}, nil
}

func (p *deschedulerPass) run(ctx context.Context, archive []byte) error {
	if err := p.unpack(ctx, archive); err != nil {
		return err
	}
	if err := engineRequest(ctx, p.endpoint, http.MethodPost, "/containers/"+p.identifier+"/start", nil, nil); err != nil {
		return fmt.Errorf("start the Descheduler pass %s: %w", p.name, err)
	}

	return p.awaitExit(ctx)
}

// The engine unpacks the archive into the created container, so no file of
// the pass exists on the host.
func (p *deschedulerPass) unpack(ctx context.Context, archive []byte) error {
	unpacking, cancel := context.WithTimeout(ctx, engineTimeout)
	defer cancel()

	query := url.Values{"path": {"/"}}
	answer, err := engineStream(unpacking, p.endpoint, http.MethodPut, "/containers/"+p.identifier+"/archive?"+query.Encode(), bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("unpack the kubeconfig and policy into the Descheduler pass %s: %w", p.name, err)
	}

	return answer.Close()
}

// A pass that errors without exiting is held to the lab's waiting time, as a
// Wait is. Either failure is explained by the pass's last log lines.
func (p *deschedulerPass) awaitExit(ctx context.Context) error {
	waiting, cancel := context.WithTimeout(ctx, scenario.WaitingTime)
	defer cancel()

	status, err := p.exitStatus(waiting)
	switch {
	case err != nil && ctx.Err() == nil && waiting.Err() != nil:
		return fmt.Errorf("the Descheduler pass %s did not end within %s; its last log lines:\n%s",
			p.name, scenario.WaitingTime, p.lastLogLines(context.WithoutCancel(ctx)))
	case err != nil:
		return err
	case status != 0:
		return fmt.Errorf("the Descheduler pass %s exited with status %d; its last log lines:\n%s",
			p.name, status, p.lastLogLines(ctx))
	default:
		return nil
	}
}

// The engine answers the wait only once the container stopped, so the answer
// is read under the caller's deadline rather than the engine timeout.
func (p *deschedulerPass) exitStatus(ctx context.Context) (int, error) {
	query := url.Values{"condition": {"not-running"}}
	answer, err := engineStream(ctx, p.endpoint, http.MethodPost, "/containers/"+p.identifier+"/wait?"+query.Encode(), nil)
	if err != nil {
		return 0, fmt.Errorf("wait for the Descheduler pass %s to end: %w", p.name, err)
	}
	defer func() { _ = answer.Close() }()

	var exited struct {
		StatusCode int
	}
	if err := json.NewDecoder(answer).Decode(&exited); err != nil {
		return 0, fmt.Errorf("read how the Descheduler pass %s ended: %w", p.name, err)
	}

	return exited.StatusCode, nil
}

// A log that cannot be read is named in place of the lines, since the error it
// explains is the one the caller acts on.
func (p *deschedulerPass) lastLogLines(ctx context.Context) string {
	reading, cancel := context.WithTimeout(ctx, engineTimeout)
	defer cancel()

	query := url.Values{"stdout": {"true"}, "stderr": {"true"}, "tail": {strconv.Itoa(lastLogLines)}}
	logged, err := engineStream(reading, p.endpoint, http.MethodGet, "/containers/"+p.identifier+"/logs?"+query.Encode(), nil)
	if err != nil {
		return fmt.Sprintf("(its log could not be read: %v)", err)
	}
	defer func() { _ = logged.Close() }()

	lines, err := lastLinesOf(io.LimitReader(logged, logBytesReadAtMost), lastLogLines)
	if err != nil {
		return fmt.Sprintf("(its log could not be read: %v)", err)
	}

	return strings.Join(lines, "\n")
}

func (p *deschedulerPass) remove(ctx context.Context) error {
	return removeContainer(ctx, p.endpoint, p.identifier, "the Descheduler pass "+p.name)
}

// lastLinesOf reads the engine's log frames and keeps the last lines of both
// streams in the order they were written. A frame the read limit cut ends the
// lines read.
func lastLinesOf(frames io.Reader, most int) ([]string, error) {
	var lines []string
	header := make([]byte, logFrameHeaderLength)
	for {
		if _, err := io.ReadFull(frames, header); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return lines, nil
			}

			return nil, fmt.Errorf("read a log frame's header: %w", err)
		}

		payload := make([]byte, binary.BigEndian.Uint32(header[logFrameLengthAt:]))
		if _, err := io.ReadFull(frames, payload); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return lines, nil
			}

			return nil, fmt.Errorf("read a log frame: %w", err)
		}

		scanned := bufio.NewScanner(bytes.NewReader(payload))
		for scanned.Scan() {
			lines = append(lines, scanned.Text())
		}
		if len(lines) > most {
			lines = lines[len(lines)-most:]
		}
	}
}
