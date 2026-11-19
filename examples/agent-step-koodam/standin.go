package agentstep

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
)

// StandIn296 is a stand-in for the Agent fields of koodam#296: spec.output,
// status.runID and status.output. Koodam v0.3.0 does not have them, and its
// CRD refuses spec.output. The first koodam release with the fields replaces
// this type, and then the seam test uses the Cluster directly.
//
// The stand-in removes spec.output from the manifest. After the run ends, it
// adds status.runID, which is the UID of the Agent as in koodam#296, and
// status.output. It computes the digest from the standard output of the agent
// container. Thus the digest check of the adapter does not test koodam while
// this stand-in is in use.
type StandIn296 struct {
	Cluster
	mu    sync.Mutex
	paths map[string]string
}

func (s *StandIn296) Create(manifest []byte) ([]byte, error) {
	var object map[string]any
	if err := json.Unmarshal(manifest, &object); err != nil {
		return nil, err
	}
	spec, _ := object["spec"].(map[string]any)
	output, _ := spec["output"].(map[string]any)
	outputPath, _ := output["path"].(string)
	if outputPath == "" {
		return nil, errors.New("the manifest has no spec.output.path")
	}
	delete(spec, "output")
	changed, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	created, err := s.Cluster.Create(changed)
	if err != nil {
		return nil, err
	}
	var meta struct {
		Metadata struct{ Namespace, Name string } `json:"metadata"`
	}
	if err := json.Unmarshal(created, &meta); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.paths == nil {
		s.paths = map[string]string{}
	}
	s.paths[meta.Metadata.Namespace+"/"+meta.Metadata.Name] = outputPath
	return created, nil
}

func (s *StandIn296) Agent(namespace, name string) ([]byte, error) {
	raw, err := s.Cluster.Agent(namespace, name)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	metadata, _ := object["metadata"].(map[string]any)
	status, _ := object["status"].(map[string]any)
	if status == nil {
		return raw, nil
	}
	status["runID"] = metadata["uid"]
	s.mu.Lock()
	outputPath := s.paths[namespace+"/"+name]
	s.mu.Unlock()
	job, _ := status["job"].(string)
	switch status["phase"] {
	case "Succeeded":
		if log, err := s.Cluster.Output(namespace, job); err != nil {
			status["output"] = map[string]any{"path": outputPath, "reason": "ReadError"}
		} else {
			sum := sha256.Sum256(log)
			status["output"] = map[string]any{"path": outputPath, "sha256": hex.EncodeToString(sum[:]), "size": len(log)}
		}
	case "Failed":
		status["output"] = map[string]any{"path": outputPath, "reason": "NoResult"}
	}
	return json.Marshal(object)
}
