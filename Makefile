
all: build/cloudwatch-metrics-publisher.json

clean:
	-rm build/*

build/cloudwatch-metrics-publisher.json: templates/cloudformation.yml
	-mkdir -p build/
	cfoo $^ > $@

build/collect-metrics.zip: $(shell find functions/collect-metrics)
	apex build collect-metrics > build/collect-metrics.zip

build/lambda-timer.zip: $(shell find functions/lambda-timer)
	apex build lambda-timer > build/lambda-timer.zip

upload: build/cloudwatch-metrics-publisher.json build/lambda-timer.zip build/collect-metrics.zip
	aws s3 sync --acl public-read build s3://buildkite-cloudwatch-metrics-publisher/

create-stack:build/cloudwatch-metrics-publisher.json
	aws cloudformation create-stack \
	--output text \
	--stack-name bk-metrics-$(shell date +%Y-%m-%d-%H-%M) \
	--disable-rollback \
	--template-body "file://${PWD}/build/cloudwatch-metrics-publisher.json" \
	--capabilities CAPABILITY_IAM \
	--parameters ParameterKey=BuildkiteApiAccessToken,ParameterValue=${BUILDKITE_API_ACCESS_TOKEN} \
		ParameterKey=BuildkiteOrgSlug,ParameterValue=${BUILDKITE_ORG_SLUG}