# Buildkite Cloudwatch Metrics Publisher

Publish [Buildkite](https://buildkite.com/) job queue statistics to [AWS Cloudwatch](http://aws.amazon.com/cloudwatch/) for easy EC2 auto-scaling of your build agents.

## Installing

<TODO>

## Developing

Development is done with [Apex](https://github.com/apex/apex) and AWS Lambda. Once you have a [Buildkite API Access Token](https://buildkite.com/user/api-access-tokens) with `read_projects` permissions created, deploy the lambda function:

```bash
apex -e BUILDKITE_ORG_SLUG=<org here> -e BUILDKITE_API_ACCESS_TOKEN=<token here> deploy
```

## License

See [LICENSE.md](LICENSE.md) (MIT)
