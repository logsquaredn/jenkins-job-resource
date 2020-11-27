package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	goqs "github.com/google/go-querystring/query"
	"github.com/logsquaredn/jenkins-job-resource"
	"github.com/yosida95/golang-jenkins"
)

type Out struct {
	stdin  io.Reader
	stderr io.Writer
	stdout io.Writer
	args   []string
}

func NewOut(
	stdin io.Reader,
	stderr io.Writer,
	stdout io.Writer,
	args []string,
) *Out {
	return &Out{
		stdin:  stdin,
		stderr: stderr,
		stdout: stdout,
		args:   args,
	}
}

const defaultCause = "Default cause"

func (o *Out) Execute() error {
	var req resource.OutRequest

	decoder := json.NewDecoder(o.stdin)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&req)
	if err != nil {
		return fmt.Errorf("invalid payload: %s", err)
	}

	jenkins := gojenkins.NewJenkins(
		&gojenkins.Auth{
			Username: req.Source.Username,
			ApiToken: req.Source.APIToken,
		},
		req.Source.URL,
	)

	params, err := goqs.Values(req.Params.BuildParams)
	if err != nil {
		return fmt.Errorf("unable to turn build_params into a query string: %s", err)
	}

	if req.Params.Cause != "" {
		params.Set("cause", req.Params.Cause)
	} else {
		params.Set("cause", defaultCause)
	}

	if req.Source.Token != "" {
		params.Set("token", req.Source.Token)
	} else {
		return fmt.Errorf("no token supplied to source")
	}

	job, err := jenkins.GetJob(req.Source.Job)
	if err != nil {
		return fmt.Errorf("unable to find job %s: %s", req.Source.Job, err)
	}

	err = jenkins.Build(job, params)
	if err != nil {
		return fmt.Errorf("unable to trigger build payload: %s", err)
	}

	var resp resource.OutResponse

	for lastBuild, err := jenkins.GetLastBuild(job); resp.Version.Number == 0 ; lastBuild, err = jenkins.GetLastBuild(job) {
		if err != nil {
			return fmt.Errorf("unable to find job %s after triggering build: %s", req.Source.Job, err)
		}
		
		if lastBuild.Number > job.LastCompletedBuild.Number {
			resp.Version = resource.ToVersion(&lastBuild)
			resp.Metadata = []resource.Metadata{
				{ Name: "description", Value: lastBuild.Description },
				{ Name: "displayName", Value: lastBuild.FullDisplayName },
				{ Name: "id", Value: lastBuild.Id },
				{ Name: "url", Value: lastBuild.Url },
				{ Name: "duration", Value: strconv.Itoa(lastBuild.Duration) },
				{ Name: "estimatedDuration", Value: strconv.Itoa(lastBuild.EstimatedDuration) },
			}
		} else {
			time.Sleep(5 * time.Second)
		}
	}

	err = json.NewEncoder(o.stdout).Encode(resp)
	if err != nil {
		return fmt.Errorf("could not marshal JSON: %s", err)
	}

	return nil
}
