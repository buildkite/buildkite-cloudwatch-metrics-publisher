package main

import (
	"flag"
	"log"
	"time"

	"github.com/99designs/go-buildkite/buildkite"
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
	result, err := collectMetrics(orgSlug, accessToken, historical)
	if err != nil {
		return err
	}

	log.Printf("Extracting cloudwatch metrics from results")
	metrics := result.toMetrics()

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

const (
	runningBuildsCount   = "RunningBuildsCount"
	runningJobsCount     = "RunningJobsCount"
	scheduledBuildsCount = "ScheduledBuildsCount"
	scheduledJobsCount   = "ScheduledJobsCount"
)

type counts map[string]int

func newCounts() counts {
	return counts{
		runningBuildsCount:   0,
		scheduledBuildsCount: 0,
		runningJobsCount:     0,
		scheduledJobsCount:   0,
	}
}

func (c counts) toMetrics(dimensions []*cloudwatch.Dimension) []*cloudwatch.MetricDatum {
	m := []*cloudwatch.MetricDatum{}

	for k, v := range c {
		m = append(m, &cloudwatch.MetricDatum{
			MetricName: aws.String(k),
			Dimensions: dimensions,
			Value:      aws.Float64(float64(v)),
			Unit:       aws.String("Count"),
		})
	}

	return m
}

type result struct {
	totals            counts
	queues, pipelines map[string]counts
}

func (r *result) toMetrics() []*cloudwatch.MetricDatum {
	data := []*cloudwatch.MetricDatum{}
	data = append(data, r.totals.toMetrics(nil)...)

	for name, c := range r.queues {
		data = append(data, c.toMetrics([]*cloudwatch.Dimension{
			{Name: aws.String("Queue"), Value: aws.String(name)},
		})...)
	}

	for name, c := range r.pipelines {
		data = append(data, c.toMetrics([]*cloudwatch.Dimension{
			{Name: aws.String("Pipeline"), Value: aws.String(name)},
		})...)
	}

	return data
}

func collectMetrics(orgSlug, accessToken string, historical time.Duration) (*result, error) {
	totals := newCounts()
	queues := map[string]counts{}
	pipelines := map[string]counts{}

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
		return nil, err
	}

	for _, queue := range builds.Queues() {
		queues[queue] = newCounts()
	}

	for _, build := range builds {
		pipelines[build.Pipeline.Name] = newCounts()
	}

	states := []string{"scheduled", "running"}

	for _, state := range states {
		builds, err := buildkite.Builds(&buildkite.BuildsInput{
			OrgSlug:  orgSlug,
			ApiToken: accessToken,
			State:    state,
		})
		if err != nil {
			return nil, err
		}

		for _, build := range builds {
			log.Printf("Adding build to stats (id=%q, pipeline=%q, branch=%q, state=%q)",
				build.ID, build.Pipeline.Name, build.Branch, build.State)

			if _, ok := pipelines[build.Pipeline.Name]; !ok {
				pipelines[build.Pipeline.Name] = newCounts()
			}

			switch build.State {
			case "running":
				totals[runningBuildsCount]++
				pipelines[build.Pipeline.Name][runningBuildsCount]++

			case "scheduled":
				totals[scheduledBuildsCount]++
				pipelines[build.Pipeline.Name][scheduledBuildsCount]++
			}

			var buildQueues = map[string]int{}

			for _, job := range build.Jobs {
				log.Printf("Adding job to stats (id=%q, pipeline=%q, queue=%q, type=%q, state=%q)",
					job.ID, build.Pipeline.Name, job.Queue(), job.Type, job.State)

				if _, ok := queues[job.Queue()]; !ok {
					queues[job.Queue()] = newCounts()
				}

				switch job.State {
				case "running":
					totals[runningJobsCount]++
					queues[job.Queue()][runningJobsCount]++

				case "scheduled":
					totals[scheduledJobsCount]++
					queues[job.Queue()][scheduledJobsCount]++
				}

				buildQueues[job.Queue()]++
			}

			// add build metrics to queues
			if len(buildQueues) > 0 {
				for queue := range buildQueues {
					switch build.State {
					case "running":
						queues[queue][runningBuildsCount]++

					case "scheduled":
						queues[queue][scheduledBuildsCount]++
					}
				}
			}
		}
	}

	return &result{totals, queues, pipelines}, nil
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
