// Copyright 2026 fanjia1024
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-resty/resty/v2"
)

func apiBaseURL() string {
	if u := os.Getenv("CORAG_API_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}

func newClient() *resty.Client {
	return resty.New().
		SetBaseURL(apiBaseURL()).
		SetTimeout(30 * time.Second).
		SetHeader("Content-Type", "application/json")
}

func getJob(jobID string) (map[string]interface{}, error) {
	var out map[string]interface{}
	resp, err := newClient().R().
		SetResult(&out).
		Get("/api/jobs/" + jobID)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("GET /api/jobs/%s: %s", jobID, resp.String())
	}
	return out, nil
}

func getJobTrace(jobID string) (map[string]interface{}, error) {
	var out map[string]interface{}
	resp, err := newClient().R().
		SetResult(&out).
		Get("/api/jobs/" + jobID + "/trace")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("GET trace: %s", resp.String())
	}
	return out, nil
}

func listAgentJobs(agentID string) ([]map[string]interface{}, error) {
	var out struct {
		Jobs []map[string]interface{} `json:"jobs"`
	}
	resp, err := newClient().R().
		SetResult(&out).
		Get("/api/agents/" + agentID + "/jobs")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("GET /api/agents/%s/jobs: %s", agentID, resp.String())
	}
	return out.Jobs, nil
}

func createAgent(name string) (string, error) {
	body := map[string]string{"name": name}
	if name == "" {
		body["name"] = "default"
	}
	var out struct {
		ID string `json:"id"`
	}
	resp, err := newClient().R().
		SetBody(body).
		SetResult(&out).
		Post("/api/agents/")
	if err != nil {
		return "", err
	}
	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
		return "", fmt.Errorf("POST /api/agents: %s", resp.String())
	}
	return out.ID, nil
}

func postMessage(agentID, message string) (jobID string, err error) {
	body := map[string]string{"message": message}
	var out struct {
		JobID string `json:"job_id"`
	}
	resp, err := newClient().R().
		SetBody(body).
		SetResult(&out).
		Post("/api/agents/" + agentID + "/message")
	if err != nil {
		return "", err
	}
	if resp.StatusCode() != http.StatusAccepted && resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("POST message: %s", resp.String())
	}
	return out.JobID, nil
}

func listTools() ([]map[string]interface{}, error) {
	var out struct {
		Tools []map[string]interface{} `json:"tools"`
	}
	resp, err := newClient().R().
		SetResult(&out).
		Get("/api/tools/")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("GET /api/tools: %s", resp.String())
	}
	return out.Tools, nil
}

func tracePageURL(jobID string) string {
	return apiBaseURL() + "/api/jobs/" + jobID + "/trace/page"
}

func getJobEvents(jobID string) (map[string]interface{}, error) {
	var out map[string]interface{}
	resp, err := newClient().R().
		SetResult(&out).
		Get("/api/jobs/" + jobID + "/events")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("GET events: %s", resp.String())
	}
	return out, nil
}

func listWorkers() ([]string, error) {
	var out struct {
		Workers []string `json:"workers"`
	}
	resp, err := newClient().R().
		SetResult(&out).
		Get("/api/system/workers")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("GET /api/system/workers: %s", resp.String())
	}
	return out.Workers, nil
}

func cancelJob(jobID string) (map[string]interface{}, error) {
	var out map[string]interface{}
	resp, err := newClient().R().
		SetResult(&out).
		Post("/api/jobs/" + jobID + "/stop")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("POST stop: %s", resp.String())
	}
	return out, nil
}

func prettyJSON(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
