package main

import (
	"flag"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/buildkite/buildkite-cloudwatch-metrics-publisher/buildkite"
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
	var (
		accessToken = flag.String("token", "", "A Buildkite API Access Token")
		orgSlug     = flag.String("org", "", "A Buildkite Organization Slug")
		interval    = flag.Duration("interval", 0, "Update metrics every interval, rather than once")
	)

	flag.Parse()

	if *accessToken == "" {
		log.Fatal("Must provide a value for -token")
	}

	if *orgSlug == "" {
		log.Fatal("Must provide a value for -org")
	}

	if err := runCollector(*orgSlug, *accessToken, time.Hour*24); err != nil {
		log.Fatal(err)
	}

	if *interval > 0 {
		for _ = range time.NewTicker(*interval).C {
			if err := runCollector(*orgSlug, *accessToken, time.Hour); err != nil {
				log.Println(err)
			}
		}
	}
}

func runCollector(orgSlug, accessToken string, historical time.Duration) error {
	svc := cloudwatch.New(session.New())

	log.Printf("Collecting buildkite metrics from org %s", orgSlug)
	result, err := collectResults(orgSlug, accessToken, historical)
	if err != nil {
		return err
	}

	log.Printf("Extracting cloudwatch metrics from results")
	metrics := result.extractMetricData()

	for _, chunk := range chunkMetricData(10, metrics) {
		log.Printf("Submitting chunk of %d metrics to Cloudwatch", len(chunk))
		if err := putMetricData(svc, chunk); err != nil {
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func collectResults(orgSlug, accessToken string, historical time.Duration) (*Result, error) {
	var res *Result = &Result{
		Queues:    map[string]Counts{},
		Pipelines: map[string]Counts{},
	}

	// Algorithm:
	// Get Builds with finished_from = 24 hours ago
	// Build results with zero values for pipelines/queues
	// Get all running and scheduled builds, add to results

	builds, err := buildkite.Builds(&buildkite.BuildsInput{
		OrgSlug:      orgSlug,
		ApiToken:     accessToken,
		FinishedFrom: time.Now().UTC().Add(historical * -1),
	})
	if err != nil {
		return res, err
	}

	for _, queue := range builds.Queues() {
		res.Queues[queue] = Counts{}
	}

	for _, build := range builds {
		if _, ok := res.Pipelines[build.Pipeline.Name]; !ok {
			res.Pipelines[build.Pipeline.Name] = Counts{}
		}
	}

	states := []string{"scheduled", "running"}

	for _, state := range states {
		builds, err := buildkite.Builds(&buildkite.BuildsInput{
			OrgSlug:  orgSlug,
			ApiToken: accessToken,
			State:    state,
		})
		if err != nil {
			return res, err
		}

		for _, build := range builds {
			log.Printf("Adding build to stats (id=%q, pipeline=%q, branch=%q, state=%q)",
				build.ID, build.Pipeline.Name, build.Branch, build.State)

			res.Counts = res.Counts.addBuild(build)
			res.Pipelines[build.Pipeline.Name] = res.Pipelines[build.Pipeline.Name].addBuild(build)

			var buildQueues = map[string]int{}

			for _, job := range build.Jobs {
				log.Printf("Adding job to stats (id=%q, pipeline=%q, queue=%q, type=%q, state=%q)",
					job.ID, build.Pipeline.Name, job.Queue(), job.Type, job.State)

				res.Counts = res.Counts.addJob(job)
				res.Pipelines[build.Pipeline.Name] = res.Pipelines[build.Pipeline.Name].addJob(job)
				res.Queues[job.Queue()] = res.Queues[job.Queue()].addJob(job)
				buildQueues[job.Queue()]++
			}

			if len(buildQueues) > 0 {
				for queue := range buildQueues {
					log.Printf("Adding stats for build to queue %s", queue)
					res.Queues[queue] = res.Queues[queue].addBuild(build)
				}
			}
		}
	}

	return res, nil
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

func (r *Result) extractMetricData() []*cloudwatch.MetricDatum {
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
