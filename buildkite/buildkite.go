package buildkite

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/tent/http-link-go"
)

const (
	dateFormat = "2006-01-02T15:04:05Z"
)

var queuePattern *regexp.Regexp

func init() {
	queuePattern = regexp.MustCompile(`(?i)^queue=(.+?)$`)
}

type Agent struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	URL      string   `json:"url"`
	Metadata []string `json:"meta_data"`
}

type Provider struct {
	ID         string `json:"id"`
	WebhookURL string `json:"webhook_url"`
}

type Creator struct {
	AvatarURL string     `json:"avatar_url"`
	CreatedAt *time.Time `json:"created_at"`
	Email     string     `json:"email"`
	ID        string     `json:"id"`
	Name      string     `json:"name"`
}

type Job struct {
	Agent           Agent      `json:"agent"`
	AgentQueryRules []string   `json:"agent_query_rules"`
	ArtifactPaths   string     `json:"artifact_paths"`
	Command         string     `json:"command"`
	CreatedAt       *time.Time `json:"created_at"`
	ExitStatus      int        `json:"exit_status"`
	FinishedAt      *time.Time `json:"finished_at"`
	ID              string     `json:"id"`
	LogURL          string     `json:"log_url"`
	Name            string     `json:"name"`
	RawLogURL       string     `json:"raw_log_url"`
	ScheduledAt     *time.Time `json:"scheduled_at"`
	StartedAt       *time.Time `json:"started_at"`
	State           string     `json:"state"`
	Type            string     `json:"type"`
	WebURL          string     `json:"web_url"`
}

// parses the the target queue based on queue=xyz in the AgentQueryRules
func (j Job) Queue() string {
	for _, m := range j.AgentQueryRules {
		if match := queuePattern.FindStringSubmatch(m); match != nil {
			return match[1]
		}
	}
	return "default"
}

func (j Job) UpdatedAt() time.Time {
	switch {
	case j.FinishedAt != nil:
		return *j.FinishedAt
	case j.StartedAt != nil:
		return *j.StartedAt
	case j.ScheduledAt != nil:
		return *j.ScheduledAt
	case j.CreatedAt != nil:
		return *j.CreatedAt
	}
	return time.Time{}
}

type Pipeline struct {
	BuildsURL            string     `json:"builds_url"`
	CreatedAt            *time.Time `json:"created_at"`
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Provider             Provider   `json:"provider"`
	Repository           string     `json:"repository"`
	RunningBuildsCount   int        `json:"running_builds_count"`
	RunningJobsCount     int        `json:"running_jobs_count"`
	ScheduledBuildsCount int        `json:"scheduled_builds_count"`
	ScheduledJobsCount   int        `json:"scheduled_jobs_count"`
	Slug                 string     `json:"slug"`
	URL                  string     `json:"url"`
	WaitingJobsCount     int        `json:"waiting_jobs_count"`
	WebURL               string     `json:"web_url"`
}

type Build struct {
	Branch      string                 `json:"branch"`
	Commit      string                 `json:"commit"`
	CreatedAt   *time.Time             `json:"created_at"`
	Creator     Creator                `json:"creator"`
	Env         map[string]interface{} `json:"env"`
	FinishedAt  *time.Time             `json:"finished_at"`
	ID          string                 `json:"id"`
	Jobs        []Job                  `json:"jobs"`
	Message     string                 `json:"message"`
	MetaData    map[string]interface{} `json:"meta_data"`
	Number      int                    `json:"number"`
	Pipeline    Pipeline               `json:"pipeline"`
	ScheduledAt *time.Time             `json:"scheduled_at"`
	Source      string                 `json:"source"`
	StartedAt   *time.Time             `json:"started_at"`
	State       string                 `json:"state"`
	URL         string                 `json:"url"`
	WebURL      string                 `json:"web_url"`
}

func (b Build) UpdatedAt() time.Time {
	switch {
	case b.FinishedAt != nil:
		return *b.FinishedAt
	case b.StartedAt != nil:
		return *b.StartedAt
	case b.ScheduledAt != nil:
		return *b.ScheduledAt
	}
	return *b.CreatedAt
}

func nextLink(linkheader string) (*url.URL, error) {
	links, err := link.Parse(linkheader)
	if err != nil {
		return nil, err
	}

	for _, link := range links {
		if link.Rel == "next" {
			return url.Parse(link.URI)
		}
	}

	return nil, nil
}

func paginate(req *http.Request, f func(resp *http.Response) error) error {
	log.Println("Querying", req.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Failed to request %s", req.URL)
	}

	err = f(resp)
	if err != nil {
		return err
	}

	next, err := nextLink(resp.Header.Get("Link"))
	if err != nil {
		return err
	}

	if next != nil {
		req.URL = next
		return paginate(req, f)
	}

	return nil
}

type BuildsInput struct {
	OrgSlug                              string
	ApiToken                             string
	CreatedFrom, CreatedTo, FinishedFrom time.Time
	State                                string
}

func (i *BuildsInput) URL() (*url.URL, error) {
	u, err := url.Parse(fmt.Sprintf(
		"https://api.buildkite.com/v2/organizations/%s/builds?per_page=100&page=1",
		i.OrgSlug,
	))
	if err != nil {
		return u, err
	}

	v := u.Query()

	if !i.CreatedFrom.IsZero() {
		v.Set("created_from", i.CreatedFrom.Format(dateFormat))
	}

	if !i.CreatedTo.IsZero() {
		v.Set("created_to", i.CreatedTo.Format(dateFormat))
	}

	if !i.FinishedFrom.IsZero() {
		v.Set("finished_from", i.FinishedFrom.Format(dateFormat))
	}

	if i.State != "" {
		v.Set("state", i.State)
	}

	u.RawQuery = v.Encode()
	return u, nil
}

func Builds(input *BuildsInput) (builds BuildSlice, err error) {
	u, err := input.URL()
	if err != nil {
		return builds, err
	}

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return builds, err
	}

	// https://buildkite.com/docs/api#authentication
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", input.ApiToken))

	timer := time.Now()
	err = paginate(req, func(resp *http.Response) error {
		var page []Build
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			return err
		}
		builds = append(builds, page...)
		return nil
	})

	if err != nil {
		return builds, err
	}

	log.Printf("Returned %d builds in %s", len(builds), time.Now().Sub(timer))
	return builds, nil
}

type BuildSlice []Build

// Queues returns unique queue names withing the Build
func (builds BuildSlice) Queues() []string {
	queueMap := map[string]struct{}{}

	for _, b := range builds {
		for _, j := range b.Jobs {
			queueMap[j.Queue()] = struct{}{}
		}
	}

	queues := []string{}

	for q := range queueMap {
		queues = append(queues, q)
	}

	return queues
}
