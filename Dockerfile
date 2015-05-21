FROM gliderlabs/alpine:3.1

RUN apk --update add bash jq curl python py-pip \
    && pip install awscli \
    && apk del py-pip \
    && apk del py-setuptools \
    && rm -rf /var/cache/apk/* \
    && rm -rf /tmp/*

ADD buildkite-cloudwatch-metrics-publisher /usr/bin/

ENTRYPOINT ["buildkite-cloudwatch-metrics-publisher"]
