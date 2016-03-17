package main

import (
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/apex/go-apex"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/buildkite/buildkite-cloudwatch-metrics-publisher/functions/collect-metrics/buildkite"
)

var historicalData = time.Hour * -72

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

		cutoff := time.Now().UTC().Add(time.Minute * -5)

		log.Printf("Querying buildkite for builds for org %s after %s",
			conf.BuildkiteOrgSlug,
			time.Now().UTC().Add(historicalData).String())

		builds, err := buildkite.Builds(&buildkite.BuildsInput{
			OrgSlug:     conf.BuildkiteOrgSlug,
			ApiToken:    conf.BuildkiteApiAccessToken,
			CreatedFrom: time.Now().UTC().Add(historicalData),
			CreatedTo:   time.Now().UTC(),
		})
		if err != nil {
			return nil, err
		}

		var res Result = Result{
			Queues:    map[string]Counts{"default": Counts{}},
			Pipelines: map[string]Counts{},
		}

		log.Printf("Aggregating results from %d builds", len(builds.Builds))

		for _, build := range builds.Builds {
			if _, ok := res.Pipelines[build.Pipeline.Name]; !ok {
				res.Pipelines[build.Pipeline.Name] = Counts{}
			}

			if afterTime(cutoff, build.CreatedAt, build.FinishedAt) {
				log.Printf("Adding build %s to stats", build.ID)
				res.Counts = res.Counts.addBuild(build)
				res.Pipelines[build.Pipeline.Name] = res.Pipelines[build.Pipeline.Name].addBuild(build)
			}

			var buildQueues = map[string]int{}
			for _, job := range build.Jobs {
				if _, ok := res.Queues[job.Queue()]; !ok {
					res.Queues[job.Queue()] = Counts{}
				}

				if afterTime(cutoff, job.CreatedAt, job.FinishedAt, job.ScheduledAt, job.StartedAt) {
					log.Printf("Adding job %s to stats", job.ID)
					res.Counts = res.Counts.addJob(job)
					res.Pipelines[build.Pipeline.Name] = res.Pipelines[build.Pipeline.Name].addJob(job)
					res.Queues[job.Queue()] = res.Queues[job.Queue()].addJob(job)
					buildQueues[job.Queue()]++
				}

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

func afterTime(after time.Time, times ...*time.Time) bool {
	for _, t := range times {
		if t != nil && t.After(after) {
			return true
		}
	}
	return false
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

	for name, c := range r.Queues {
		data = append(data, c.asMetrics([]*cloudwatch.Dimension{
			{Name: aws.String("Queue"), Value: aws.String(name)},
		})...)
	}

	for name, c := range r.Pipelines {
		data = append(data, c.asMetrics([]*cloudwatch.Dimension{
			{Name: aws.String("Pipeline"), Value: aws.String(name)},
		})...)
	}

	return data
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
