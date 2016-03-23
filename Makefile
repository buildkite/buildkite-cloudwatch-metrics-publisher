
all: build/cloudwatch-metrics-publisher.json

clean:
	-rm build/*

build/cloudwatch-metrics-publisher.json: templates/cloudformation.yml
	-mkdir -p build/
	cfoo $^ > $@

build/buildkite-cloudwatch-metrics:
	-mkdir -p build/
	which glide || go get github.com/Masterminds/glide
	glide install
	go build -o build/buildkite-cloudwatch-metrics ./cli/buildkite-cloudwatch-metrics/

build: build/cloudwatch-metrics-publisher.json build/buildkite-cloudwatch-metrics

upload: build
	aws s3 sync --acl public-read build s3://buildkite-cloudwatch-metrics-publisher/

create-stack: build/cloudwatch-metrics-publisher.json
	aws cloudformation create-stack \
	--output text \
	--stack-name buildkite-metrics-$(shell date +%Y-%m-%d-%H-%M) \
	--disable-rollback \
	--template-body "file://${PWD}/build/cloudwatch-metrics-publisher.json" \
	--capabilities CAPABILITY_IAM \
	--parameters ParameterKey=BuildkiteApiAccessToken,ParameterValue=$(token) \
		ParameterKey=BuildkiteOrgSlug,ParameterValue=$(org) \
		ParameterKey=KeyName,ParameterValue=$(keyname)

validate: build/cloudwatch-metrics-publisher.json
	aws cloudformation validate-template \
	--output table \
	--template-body "file://${PWD}/build/cloudwatch-metrics-publisher.json"
