FROM alpine:3.7

ARG GITHUB_TOKEN
ARG SERVICE
ENV GOPATH=/go

WORKDIR /go/src/github.com/utilitywarehouse/${SERVICE}
ADD . /go/src/github.com/utilitywarehouse/${SERVICE}/

RUN apk update \
  && apk add make git go gcc musl-dev --no-cache ca-certificates \
  && git config --global url."https://${GITHUB_TOKEN}:x-oauth-basic@github.com/".insteadOf "https://github.com/" \
  && make clean install \
  && make ${SERVICE} \
  && mv ${SERVICE} /${SERVICE} \
  && mkdir /templates

COPY ./cmd/${SERVICE}/templates/* /templates/

CMD ["/health-aggregator"]