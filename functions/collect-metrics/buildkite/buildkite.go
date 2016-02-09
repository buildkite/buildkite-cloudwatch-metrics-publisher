package buildkite

import "regexp"

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
	AvatarURL string `json:"avatar_url"`
	CreatedAt string `json:"created_at"`
	Email     string `json:"email"`
	ID        string `json:"id"`
	Name      string `json:"name"`
}

type Job struct {
	Agent           Agent    `json:"agent"`
	AgentQueryRules []string `json:"agent_query_rules"`
	ArtifactPaths   string   `json:"artifact_paths"`
	Command         string   `json:"command"`
	CreatedAt       string   `json:"created_at"`
	ExitStatus      int      `json:"exit_status"`
	FinishedAt      string   `json:"finished_at"`
	ID              string   `json:"id"`
	LogURL          string   `json:"log_url"`
	Name            string   `json:"name"`
	RawLogURL       string   `json:"raw_log_url"`
	ScheduledAt     string   `json:"scheduled_at"`
	StartedAt       string   `json:"started_at"`
	State           string   `json:"state"`
	Type            string   `json:"type"`
	WebURL          string   `json:"web_url"`
}

// parses job metadata and extracts queue=xyz
func (j Job) Queue() string {
	for _, m := range j.Agent.Metadata {
		if match := queuePattern.FindStringSubmatch(m); match != nil {
			return match[1]
		}
	}
	return "default"
}

type Pipeline struct {
	BuildsURL            string   `json:"builds_url"`
	CreatedAt            string   `json:"created_at"`
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Provider             Provider `json:"provider"`
	Repository           string   `json:"repository"`
	RunningBuildsCount   int      `json:"running_builds_count"`
	RunningJobsCount     int      `json:"running_jobs_count"`
	ScheduledBuildsCount int      `json:"scheduled_builds_count"`
	ScheduledJobsCount   int      `json:"scheduled_jobs_count"`
	Slug                 string   `json:"slug"`
	URL                  string   `json:"url"`
	WaitingJobsCount     int      `json:"waiting_jobs_count"`
	WebURL               string   `json:"web_url"`
}

type Build struct {
	Branch      string                 `json:"branch"`
	Commit      string                 `json:"commit"`
	CreatedAt   string                 `json:"created_at"`
	Creator     Creator                `json:"creator"`
	Env         map[string]interface{} `json:"env"`
	FinishedAt  string                 `json:"finished_at"`
	ID          string                 `json:"id"`
	Jobs        []Job                  `json:"jobs"`
	Message     string                 `json:"message"`
	MetaData    map[string]interface{} `json:"meta_data"`
	Number      int                    `json:"number"`
	Pipeline    Pipeline               `json:"pipeline"`
	ScheduledAt string                 `json:"scheduled_at"`
	Source      string                 `json:"source"`
	StartedAt   string                 `json:"started_at"`
	State       string                 `json:"state"`
	URL         string                 `json:"url"`
	WebURL      string                 `json:"web_url"`
}
