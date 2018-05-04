FROM golang:1.9-alpine AS build

RUN apk update && apk add make git gcc musl-dev

ARG GITHUB_TOKEN
ARG SERVICE

RUN git config --global url."https://${GITHUB_TOKEN}:x-oauth-basic@github.com/".insteadOf "https://github.com/"

ADD . /go/src/github.com/utilitywarehouse/${SERVICE}

WORKDIR /go/src/github.com/utilitywarehouse/${SERVICE}

RUN make clean install
RUN make ${SERVICE}

RUN mv ${SERVICE} /${SERVICE}
RUN mkdir /templates
COPY ./cmd/${SERVICE}/templates/* /templates

FROM alpine:3.6

ARG SERVICE

ENV APP=${SERVICE}

RUN apk add --no-cache ca-certificates && mkdir /app && mkdir /app/templates
COPY --from=build /${SERVICE} /app/${SERVICE}
COPY --from=build /templates /app/templates

ENTRYPOINT /app/${APP}