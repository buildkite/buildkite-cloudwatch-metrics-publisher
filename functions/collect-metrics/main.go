package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/apex/go-apex"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/buildkite/buildkite-cloudwatch-metrics-publisher/functions/collect-metrics/buildkite"
)

// Generates:
// Buildkite > RunningBuildsCount
// Buildkite > RunningJobsCount
// Buildkite > ScheduledBuildsCount
// Buildkite > ScheduledJobsCount
// Buildkite > (Queue) > RunningBuildsCount
// Buildkite > (Queue) > RunningJobsCount
// Buildkite > (Queue) > ScheduledBuildsCount
// Buildkite > (Queue) > ScheduledJobsCount
// Buildkite > (Pipeline) > RunningBuildsCount
// Buildkite > (Pipeline) > RunningJobsCount
// Buildkite > (Pipeline) > ScheduledBuildsCount
// Buildkite > (Pipeline) > ScheduledJobsCount

func main() {
	apex.HandleFunc(func(event json.RawMessage, ctx *apex.Context) (interface{}, error) {
		svc := cloudwatch.New(session.New())

		var conf Config
		if err := json.Unmarshal(event, &conf); err != nil {
			return nil, err
		}

		if conf.BuildkiteApiAccessToken == "" {
			return nil, errors.New("No BuildkiteApiAccessToken provided")
		}

		if conf.BuildkiteOrgSlug == "" {
			return nil, errors.New("No BuildkiteOrgSlug provided")
		}

		log.Printf("Querying buildkite for builds for org %s for past 5 mins", conf.BuildkiteOrgSlug)
		builds, err := recentBuildkiteBuilds(conf.BuildkiteOrgSlug, conf.BuildkiteApiAccessToken)
		if err != nil {
			return nil, err
		}

		var res Result = Result{
			Queues:    map[string]Counts{},
			Pipelines: map[string]Counts{},
		}

		log.Printf("Aggregating results from %d builds", len(builds))
		for _, build := range builds {
			res.Counts = res.Counts.addBuild(build)
			res.Pipelines[build.Pipeline.Name] = res.Pipelines[build.Pipeline.Name].addBuild(build)

			var buildQueues = map[string]int{}
			for _, job := range build.Jobs {
				res.Counts = res.Counts.addJob(job)
				res.Pipelines[build.Pipeline.Name] = res.Pipelines[build.Pipeline.Name].addJob(job)
				res.Queues[job.Queue()] = res.Queues[job.Queue()].addJob(job)
				buildQueues[job.Queue()]++
			}

			for queue := range buildQueues {
				res.Queues[queue] = res.Queues[queue].addBuild(build)
			}
		}

		log.Printf("Extracting cloudwatch metrics from results")
		metrics := res.extractMetricData()

		for _, chunk := range chunkMetricData(10, metrics) {
			log.Printf("Submitting chunk of %d metrics to Cloudwatch", len(chunk))
			if err = putMetricData(svc, chunk); err != nil {
				return nil, err
			}
		}

		return res, nil
	})
}

type Config struct {
	BuildkiteOrgSlug, BuildkiteApiAccessToken string
}

type Counts struct {
	RunningBuilds, RunningJobs, ScheduledBuilds, ScheduledJobs int
}

func (c Counts) addBuild(build buildkite.Build) Counts {
	switch build.State {
	case "running":
		c.RunningBuilds++
	case "scheduled":
		c.ScheduledBuilds++
	}
	return c
}

func (c Counts) addJob(job buildkite.Job) Counts {
	switch job.State {
	case "running":
		c.RunningJobs++
	case "scheduled":
		c.ScheduledJobs++
	}
	return c
}

func (c Counts) asMetrics(dimensions []*cloudwatch.Dimension) []*cloudwatch.MetricDatum {
	return []*cloudwatch.MetricDatum{
		&cloudwatch.MetricDatum{
			MetricName: aws.String("RunningBuildsCount"),
			Dimensions: dimensions,
			Value:      aws.Float64(float64(c.RunningBuilds)),
			Unit:       aws.String("Count"),
		},
		&cloudwatch.MetricDatum{
			MetricName: aws.String("ScheduledBuildsCount"),
			Dimensions: dimensions,
			Value:      aws.Float64(float64(c.ScheduledBuilds)),
			Unit:       aws.String("Count"),
		},
		&cloudwatch.MetricDatum{
			MetricName: aws.String("RunningJobsCount"),
			Dimensions: dimensions,
			Value:      aws.Float64(float64(c.RunningJobs)),
			Unit:       aws.String("Count"),
		},
		&cloudwatch.MetricDatum{
			MetricName: aws.String("ScheduledJobsCount"),
			Dimensions: dimensions,
			Value:      aws.Float64(float64(c.ScheduledJobs)),
			Unit:       aws.String("Count"),
		},
	}
}

type Result struct {
	Counts
	Queues, Pipelines map[string]Counts
}

func (r Result) extractMetricData() []*cloudwatch.MetricDatum {
	data := []*cloudwatch.MetricDatum{}
	data = append(data, r.Counts.asMetrics(nil)...)

	for name, _ := range r.Queues {
		data = append(data, r.Counts.asMetrics([]*cloudwatch.Dimension{
			{Name: aws.String("Queue"), Value: aws.String(name)},
		})...)
	}

	// write pipeline metrics, include project dimension for backwards compat
	for name, _ := range r.Pipelines {
		data = append(data, r.Counts.asMetrics([]*cloudwatch.Dimension{
			{Name: aws.String("Project"), Value: aws.String(name)},
			{Name: aws.String("Pipeline"), Value: aws.String(name)},
		})...)
	}

	return data
}

func recentBuildkiteBuilds(orgSlug, apiKey string) ([]buildkite.Build, error) {
	url := fmt.Sprintf(
		"https://api.buildkite.com/v2/organizations/%s/builds?created_from=%s&page=%d",
		orgSlug,
		time.Now().UTC().Add(time.Minute*-5).Format("2006-01-02T15:04:05Z"),
		1,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	//Issue the request and get the bearer token from the JSON you get back
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Failed to request %s", url)
	}

	// TODO: Pagination, but ain't nobody got time for that.
	// log.Printf("%#v", resp.Header.Get("Link"))

	var builds []buildkite.Build
	if err = json.NewDecoder(resp.Body).Decode(&builds); err != nil {
		return nil, err
	}

	return builds, nil
}

func chunkMetricData(size int, data []*cloudwatch.MetricDatum) [][]*cloudwatch.MetricDatum {
	var chunks = [][]*cloudwatch.MetricDatum{}
	for i := 0; i < len(data); i += size {
		end := i + size
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, data[i:end])
	}
	return chunks
}

func putMetricData(svc *cloudwatch.CloudWatch, data []*cloudwatch.MetricDatum) error {
	_, err := svc.PutMetricData(&cloudwatch.PutMetricDataInput{
		MetricData: data,
		Namespace:  aws.String("Buildkite"),
	})
	if err != nil {
		return err
	}

	return nil
}
