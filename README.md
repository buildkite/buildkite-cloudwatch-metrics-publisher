# Buildkite Cloudwatch Metrics Publisher

[![Launch BK Cloudwatch Metrics Publisher](http://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/images/cloudformation-launch-stack-button.png)](https://console.aws.amazon.com/cloudformation/home?region=us-east-1#/stacks/new?stackName=buildkite-cloudwatch-metrics-publisher&templateURL=https://s3.amazonaws.com/buildkite-cloudwatch-metrics-publisher/master/cloudwatch-metrics-publisher.json)

Publish [Buildkite](https://buildkite.com/) job queue statistics to [AWS Cloudwatch](http://aws.amazon.com/cloudwatch/) for easy EC2 auto-scaling of your build agents.

## Installing

The easiest way to install is to press the above button and then enter your org slug and your [Buildkite API Access Token](https://buildkite.com/user/api-access-tokens) with `read_projects` permissions created.

Alternately, run via the command-line:

```bash
aws cloudformation create-stack \
	--output text \
	--stack-name buildkite-cloudwatch-metrics-publisher \
	--disable-rollback \
	--template-body "https://s3.amazonaws.com/buildkite-cloudwatch-metrics-publisher/master/cloudwatch-metrics-publisher.json" \
	--capabilities CAPABILITY_IAM \
	--parameters ParameterKey=BuildkiteApiAccessToken,ParameterValue=BUILDKITE_API_TOKEN_GOES_HERE \
	ParameterKey=BuildkiteOrgSlug,ParameterValue=BUILDKITE_ORG_SLUG_GOES_HERE
```

After the stack is run you will have the following metrics populated in CloudWatch (updated every 5 minutes):

```
Buildkite > RunningBuildsCount
Buildkite > RunningJobsCount
Buildkite > ScheduledBuildsCount
Buildkite > ScheduledJobsCount
Buildkite > IdleAgentsCount
Buildkite > BusyAgentsCount
Buildkite > TotalAgentsCount

Buildkite > (Queue) > RunningBuildsCount
Buildkite > (Queue) > RunningJobsCount
Buildkite > (Queue) > ScheduledBuildsCount
Buildkite > (Queue) > ScheduledJobsCount
Buildkite > (Queue) > IdleAgentsCount
Buildkite > (Queue) > BusyAgentsCount
Buildkite > (Queue) > TotalAgentsCount

Buildkite > (Pipeline) > RunningBuildsCount
Buildkite > (Pipeline) > RunningJobsCount
Buildkite > (Pipeline) > ScheduledBuildsCount
Buildkite > (Pipeline) > ScheduledJobsCount
```

## Developing

You can build and run the binary tool locally with golang installed:

```
go run cli/buildkite-cloudwatch-metrics/*.go -org [myorg] -token [mytoken]
```

## License

See [LICENSE.md](LICENSE.md) (MIT)
