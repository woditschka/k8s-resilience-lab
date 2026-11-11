//go:build !integration

package cluster

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

// A declined value reaches a warning and the run's final error, neither of
// which may carry the password. A value is declined on two branches: one that
// parses and is redacted, and one whose url.Parse error quotes the raw value
// back.
func TestADeclinedDockerHost_ShouldBeReportedWithoutItsPassword(t *testing.T) {
	const password = "the-engine-password"
	const passwordURLParseRejects = "the-engine-pass word"

	for _, declined := range []struct {
		branch   string
		value    string
		password string
	}{
		{
			branch:   "a value that parses",
			value:    "ssh://operator:" + password + "@engine.example.invalid",
			password: password,
		},
		{
			branch:   "a value that does not parse",
			value:    "ssh://operator:" + passwordURLParseRejects + "@engine.example.invalid",
			password: passwordURLParseRejects,
		},
	} {
		t.Run(declined.branch, func(t *testing.T) {
			warnings, written := recordedWarnings()

			_, err := detectEngine(t.Context(), warnings, declined.value, []string{aSocketThatIsNotThere(t)})

			if err == nil {
				t.Fatal("neither the DOCKER_HOST value nor the candidate names an engine, but an endpoint was resolved")
			}
			if strings.Contains(written.String(), declined.password) {
				t.Errorf("the warning carries the DOCKER_HOST password, and it reads: %s", written)
			}
			if strings.Contains(err.Error(), declined.password) {
				t.Errorf("the run's error carries the DOCKER_HOST password, and it reads: %v", err)
			}
		})
	}
}

// These are the shapes whose parse error quotes part of the input back: the
// whole value in the *url.Error, three bytes at a bad percent escape, and the
// port at a bad port.
func TestAParseFailure_ShouldBeReportedWithoutTheValueThatFailed(t *testing.T) {
	fixedCause := []string{dockerHostVariable + " (it is not a URL: " + unparsableCause + ")"}

	for _, rejected := range []struct {
		shape    string
		value    string
		quotes   string
		fragment string
	}{
		{
			shape:    "a space in the password",
			value:    "ssh://operator:the-engine-pass word@engine.example.invalid",
			quotes:   "the whole value",
			fragment: "the-engine-pass",
		},
		{
			shape:    "a bad percent escape in the password",
			value:    "ssh://operator:the-engine-pass%zzword@engine.example.invalid",
			quotes:   "three bytes of it",
			fragment: "%zz",
		},
		{
			shape:    "a port that is not a number",
			value:    "tcp://engine.example.invalid:the-engine-port",
			quotes:   "the port it read",
			fragment: "the-engine-port",
		},
	} {
		t.Run(rejected.shape, func(t *testing.T) {
			warnings, written := recordedWarnings()

			_, tried, resolved := engineFromDockerHost(t.Context(), warnings, rejected.value)

			expect.Expect(t, resolved).To(expect.Equal(false))
			expect.Expect(t, tried).To(expect.Equal(fixedCause))
			if strings.Contains(written.String(), rejected.fragment) {
				t.Errorf("the parse error quotes %s, and the warning carries %q of the %s value: %s",
					rejected.quotes, rejected.fragment, dockerHostVariable, written)
			}
		})
	}
}

func TestWhenNoSocketAnswers_ShouldFailNamingEveryPathTriedAndWhy(t *testing.T) {
	firstCandidate, secondCandidate := aSocketThatIsNotThere(t), aSocketThatIsNotThere(t)
	deadDockerHost := aSocketThatIsNotThere(t)

	_, err := detectEngine(t.Context(), discardedWarnings(), "unix://"+deadDockerHost, []string{firstCandidate, secondCandidate})

	if err == nil {
		t.Fatal("no socket answering is an error, but an endpoint was resolved")
	}
	for _, tried := range []string{deadDockerHost, firstCandidate, secondCandidate} {
		if !strings.Contains(err.Error(), tried) {
			t.Errorf("the error names every path tried, but %s is missing from: %v", tried, err)
		}
	}
}

// aPullCutShortBy lacks every image, and its pull's progress stream fails with
// the error cut returns once one message is read, as a download cut part-way
// leaves it.
func aPullCutShortBy(cut func(*http.Request) error) answeredBy {
	return func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost {
			return anEngineAnswer(http.StatusNotFound, strings.NewReader(`{"message":"no such image"}`)), nil
		}

		return anEngineAnswer(http.StatusOK, io.MultiReader(strings.NewReader(`{"status":"Pulling fs layer"}`), cutBy{request, cut})), nil
	}
}

func anEngineAnswer(code int, body io.Reader) *http.Response {
	return &http.Response{StatusCode: code, Status: http.StatusText(code), Body: io.NopCloser(body)}
}

// cutBy fails the read it is reached at rather than when the answer is made,
// so the message before it is read first.
type cutBy struct {
	request *http.Request
	cut     func(*http.Request) error
}

func (c cutBy) Read([]byte) (int, error) { return 0, c.cut(c.request) }

func aReadError(*http.Request) error { return io.ErrUnexpectedEOF }

// The stream stalls until the obtain's own deadline ends it.
func theObtainTimeout(request *http.Request) error {
	<-request.Context().Done()

	return request.Context().Err()
}

// A stalled pull must end this test long before the pull's own timeout.
const anObtainTimeout = 100 * time.Millisecond

func TestAKubernetesVersionWhoseDownloadIsCutShort_ShouldNotBeObtainedAndShouldBeNamed(t *testing.T) {
	t.Parallel()
	version := DefaultKubernetesVersion()

	for cause, cut := range map[string]func(*http.Request) error{"a read error": aReadError, "the obtain timeout": theObtainTimeout} {
		t.Run(cause, func(t *testing.T) {
			obtaining, cancel := context.WithTimeout(t.Context(), anObtainTimeout)
			defer cancel()

			err := ObtainVersion(obtaining, anEngineStandIn(aPullCutShortBy(cut)), version)

			expect.Expect(t, strings.Contains(fmt.Sprint(err), version.String())).To(expect.Equal(true))
		})
	}
}

func TestADeschedulerWhoseDownloadIsCutShort_ShouldNotBeObtainedAndShouldBeNamed(t *testing.T) {
	t.Parallel()
	paired := theDeschedulerPairedByDefault(t)

	err := ObtainDescheduler(t.Context(), anEngineStandIn(aPullCutShortBy(aReadError)), paired)

	expect.Expect(t, strings.Contains(fmt.Sprint(err), paired.String())).To(expect.Equal(true))
}
