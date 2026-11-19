package agentstep

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Kubectl is a Cluster that runs the kubectl executable. The adapter module
// has no Kubernetes library.
type Kubectl struct {
	// Kubeconfig is the --kubeconfig value. It must not be empty, so that a
	// run never uses the current context of the user by accident.
	Kubeconfig string
	// Timeout is the limit of one command. Zero is 60 seconds.
	Timeout time.Duration
}

var objectName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]{0,251}[a-z0-9])?$`)

func (k Kubectl) run(stdin []byte, limit int, args ...string) ([]byte, error) {
	if k.Kubeconfig == "" {
		return nil, errors.New("kubectl needs an explicit kubeconfig")
	}
	timeout := k.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "kubectl", append([]string{"--kubeconfig", k.Kubeconfig}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = bytes.NewReader(stdin), &stdout, &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if len(message) > 400 {
			message = message[:400]
		}
		return nil, errors.New("kubectl " + args[0] + ": " + err.Error() + ": " + message)
	}
	if stdout.Len() > limit {
		return nil, errors.New("kubectl " + args[0] + ": the output is too large")
	}
	return stdout.Bytes(), nil
}

// Create runs "kubectl create". The API server validates the manifest.
func (k Kubectl) Create(manifest []byte) ([]byte, error) {
	return k.run(manifest, 1<<20, "create", "-f", "-", "-o", "json")
}

// Agent runs "kubectl get" for one Agent.
func (k Kubectl) Agent(namespace, name string) ([]byte, error) {
	if !objectName.MatchString(namespace) || !objectName.MatchString(name) {
		return nil, errors.New("the namespace or the Agent name is not valid")
	}
	return k.run(nil, 1<<20, "-n", namespace, "get", "agents.core.koodam.dev", name, "-o", "json")
}

// Output runs "kubectl logs" for the agent container of the run Job.
func (k Kubectl) Output(namespace, job string) ([]byte, error) {
	if !objectName.MatchString(namespace) || !objectName.MatchString(job) {
		return nil, errors.New("the namespace or the Job name is not valid")
	}
	return k.run(nil, maxOutput, "-n", namespace, "logs", "job/"+job, "-c", "agent")
}
