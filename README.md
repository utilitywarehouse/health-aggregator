# Health Aggregator

A service aggregating health endpoint information from our kubernetes cluster.

[![MIT licensed][shield-license]](https://github.com/utilitywarehouse/health-aggregator/blob/master/LICENSE)
[![CircleCI](https://circleci.com/gh/utilitywarehouse/health-aggregator.svg?style=svg)](https://circleci.com/gh/utilitywarehouse/health-aggregator)

## Table of Contents

* [Requirements](#requirements)
* [Usage](#usage)
* [Integrating](#integrating)
* [GUI](#gui)
* [Endpoints](#endpoints)
  * [GET /namespaces](#get-namespaces)
  * [GET /namespaces/{namespace}/services](#get-namespacesnamespaceservices)
  * [GET /services](#get-services)
  * [GET /namespaces/{namespace}/services/{service}/checks](#get-namespacesnamespaceservicesservicechecks)
  * [GET /namespaces/{namespace}/services/checks](#get-namespacesnamespaceserviceschecks)
  * [POST /reload](#post-reload)
  * [GET /kube-ops/ready](#get-kube-opsready)
* [License](#license)

## Requirements

Health Aggregator requires the following to run:

* [Golang][golang] 1.9+
* [Docker][docker]

## Usage

From the root directory, go get all dependencies:

```sh
go get ./...
```

Build, test and install:

```sh
make all
```

### Running in cluster

```sh
export KUBERNETES_SERVICE_HOST="elb.master.k8s.dev.uw.systems"
export KUBERNETES_SERVICE_PORT="8443"
```

### Other optional params

```sh
health-aggregator --help
```

```sh
      --port                       Port to listen on (env $PORT) (default "8080")
      --ops-port                   The HTTP ops port (env $OPS_PORT) (default 8081)
      --write-timeout              The WriteTimeout for HTTP connections (env $HTTP_WRITE_TIMEOUT) (default 15)
      --read-timeout               The ReadTimeout for HTTP connections (env $HTTP_READ_TIMEOUT) (default 15)
      --log-level                  Log level (e.g. INFO, DEBUG, WARN) (env $LOG_LEVEL) (default "INFO")
      --mongo-connection-string    Connection string to connect to mongo ex mongodb:27017/ (env $MONGO_CONNECTION_STRING) (default "127.0.0.1:27017/")
      --mongo-drop-db              Set to true in order to drop the DB on startup (env $MONGO_DROP_DB)
      --delete-checks-after-days   Age of check results in days after which they are deleted (env $DELETE_CHECKS_AFTER_DAYS) (default 1)
      --restrict-namespace         Restrict checks to one or more namespaces - e.g. export RESTRICT_NAMESPACE="labs","energy"
```

### Start MongoDB

```sh
docker-compose up -d
```

### Start the app

```sh
health-aggregator
```

## Integrating

It's not necessary to run your own instance of health-aggregator, although that is an option. health-aggregator can collect health check data from multiple namespaces.

### To add a new namespace without running a new instance of health aggregator

#### Step 1 - Include your namespace

Add the namespace name to the `RESTRICT_NAMESPACE` environment variable in the `health-aggregator` kubernetes manifest in the `labs` namespace for your environment.

For example:

```yaml
- name: RESTRICT_NAMESPACE
  value: smartmetering,partner-portal,energy,crm,customer-platform,jtc,customer-onboarding,insurance
```

#### Step 2 - Annotate your namespace and services

Once added and applied, health-aggregator will start to scrape the `/__/health` endpoints of all Kubernetes *Services* found in the namespace. By default, `health-aggregtor` will attempt to load the health check endpoint on port `8081`.

If the most commonly used port for the `/__/health` endpoint in your particular namespace is something else e.g. `8080`, then add the following annotation in the namespace manifest:

```yaml
---
kind: Namespace
apiVersion: v1
metadata:
  name: my-namespace
  labels:
    name: my-namespace
  annotations:
    uw.health.aggregator.port: '8080'
...
```

If there are services within your namespace that use a different port again, then add an annotation against the **Service**, like so:

```yaml
---
apiVersion: v1
kind: Service
metadata:
  annotations:
    prometheus.io/scrape: 'true'
    prometheus.io/path:   /__/metrics
    prometheus.io/port:   '8081'
    uw.health.aggregator.port: '3000'
...
```

Annotations added to the Service *override* any annotations at the namespace level.

If there are Services which either do not have a health endpoint or you do not wish for that Service to have its health endpoint scraped, you can add the following Service annotation:

```yaml
uw.health.aggregator.enable: 'false'
```

This annotation can also be applied at namespace level and would have the effect of disabling the health scraping of all Services. Only Service which have the opposite annotation value would then be scraped:

```yaml
uw.health.aggregator.enable: 'true'
```

#### Step 3 - Reload

Now that you've added annotations, force a reload. See here: [POST /reload](#post-reload).

### To add an instance of health-aggregator to your namespace

Note: you require an instance of mongo running in your cluster. This may be removed in future development.

Copy the manifest from labs and modify the following:

* The namespace name (except in the reference to `labs` in the registry URI)
* Set `RESTRICT_NAMESPACE` to your own namespace name
* Set the Ingress host as required for your instance

Then follow `Step 2 - Annotate your namespace and services` and `Step 3 - Reload` as above.

## GUI

There is an auto-refreshing GUI packaged with health-aggregator for dashboards on a per namespace and environment basis. This can be accessed like so:

```txt
https://health-aggregator.prod.uw.systems/?ns={namespace_name}&env={environment}
```

**env** can be set to either of `dev` or `prod`

E.g: https://health-aggregator.prod.uw.systems/?ns=partner-portal&env=dev

![Health Aggregator GUI](https://github.com/utilitywarehouse/health-aggregator/blob/master/health-aggregator-gui.png)

Tile colour represents the aggregated health of the Pods attached to the service. The aggregate health of the Service takes the most severe health state of all attached Pods. So, if one Pod is `unhealthy`, another `degraded` and another `healthy` then the aggregate health is `unhealthy` and as such represented by a red colour.

The Service name can be clicked on to show details such as:

* Time the last check was performed
* How long has the Service been in this state
* What the previous state was
* What status code and error (if any) did the pod health check return?
* Details of the Pod health check response including the specific checks

Depending on the size of screen dispayed on, and the number of services here are some additional query parameters than can help fit the tiles to the given view:

* `bigscreen=true` - enlarges tiles and text so as to be visible at a distance
* `compact=true` - reduces empty space to fit as many tiles in the available area (for namespaces with many services)

Both of the above query parameters can work together.

## Endpoints

### GET /namespaces

Return a list of namespaces for the cluster, including the health aggregator settings at namespace level. Namespaces are loaded at app startup or when doing a POST to `/reload`.

```json
  [
    {
      "name": "acs",
      "healthAnnotations": {
        "enableScrape": "true",
        "port": "8080"
      }
    }
  ]
```

### GET /namespaces/{namespace}/services 

Return a list of services for a given namespace, including the health aggregator settings at service level. Services are loaded at app startup or when doing a POST to `/reload`.

```json
  [
    {
      "name": "redis",
      "namespace": "auth",
      "healthcheckURL": "http://redis.auth:8080/__/health",
      "healthAnnotations": {
        "enableScrape": "true",
        "port": "8080"
      }
    }
  ]
```

### GET /services

Return a list of all services for the cluster, including the health aggregator settings at namespace level. Services are loaded at app startup or when doing a POST to `/reload`.

```json
  [
    {
      "name": "redis",
      "namespace": "auth",
      "healthcheckURL": "http://redis.auth:8080/__/health",
      "healthAnnotations": {
        "enableScrape": "true",
        "port": "8080"
      }
    }
  ]
```

### GET /namespaces/{namespace}/services/{service}/checks

Return a list of the last 50 checks for a service sort in time descending order. Checks are carried out at regular intervals as specified within the app.

```json
  [
    {
      "service": {
        "name": "uw-foo",
        "namespace": "foo-bar",
        "healthcheckURL": "http://uw-foo.foo-bar:8080/__/health",
        "healthAnnotations": {
          "enableScrape": "true",
          "port": "8080"
        }
      },
      "checkTime": "2018-04-18T10:22:10.944Z",
      "state": "unhealthy",
      "stateSince": "2018-04-18T09:45:53.931Z",
      "statusCode": 200,
      "error": "",
      "healthcheckBody": {
        "name": "uw-foo",
        "description": "Performs the foo bar baz functions",
        "health": "unhealthy",
        "checks": [
          {
            "name": "Database connectivity",
            "health": "healthy",
            "output": "connection to db1234.uw.systems is ok"
          },
          {
            "name": "Message queue connection",
            "health": "degraded",
            "output": "Connected OK to broker01.uw.systems ok\nFailed to connect to broker02.uw.systems",
            "action": "Check that the message queue on broker02.uw.systems is running and check network connectivity"
          },
          {
            "name": "SMTP server connectivity",
            "health": "unhealthy",
            "output": "failed to connect to smtp123.uw.systems on port 25 : Connection refused",
            "action": "Check the SMTP server on smtp123.uw.system is running and check network connectivity",
            "impact": "Users will not receive email notifications whenever a foo bar action is completed"
          }
        ]
      }
    }
  ]
```

### GET /namespaces/{namespace}/services/checks

Returns a list of the most recent check responses for each of the services in the specified namespace.

Default behaviour for this endpoint is to return an HTML formatted response. Use the following header for a json response:

    Accept: application/json

```json
  [
    {
      "service": {
        "name": "uw-foo",
        "namespace": "foo-bar",
        "healthcheckURL": "http://uw-foo.foo-bar:8080/__/health",
        "healthAnnotations": {
          "enableScrape": "true",
          "port": "8080"
        }
      },
      "checkTime": "2018-04-18T10:22:10.944Z",
      "state": "unhealthy",
      "stateSince": "2018-04-18T09:45:53.931Z",
      "statusCode": 200,
      "error": "",
      "healthcheckBody": {
        "name": "uw-foo",
        "description": "Performs the foo bar baz functions",
        "health": "unhealthy",
        "checks": [
          {
            "name": "Database connectivity",
            "health": "healthy",
            "output": "connection to db1234.uw.systems is ok"
          },
          {
            "name": "Message queue connection",
            "health": "degraded",
            "output": "Connected OK to broker01.uw.systems ok\nFailed to connect to broker02.uw.systems",
            "action": "Check that the message queue on broker02.uw.systems is running and check network connectivity"
          },
          {
            "name": "SMTP server connectivity",
            "health": "unhealthy",
            "output": "failed to connect to smtp123.uw.systems on port 25 : Connection refused",
            "action": "Check the SMTP server on smtp123.uw.system is running and check network connectivity",
            "impact": "Users will not receive email notifications whenever a foo bar action is completed"
          }
        ]
      }
    }
  ]
```

### POST /reload

This POST with empty body carries out the discovery process for all health endpoints once more, allowing any annotation changes or new services and namespaces to be picked up.

### GET /kube-ops/ready

This endpoint is used for the kubernetes readiness check and returns a simply 200 response code once the main http server is running.

## License

Health Aggregator is licensed under the [MIT](https://github.com/utilitywarehouse/health-aggregator/blob/master/LICENSE) license.

[golang]: https://golang.org/
[docker]: https://www.docker.com/
[shield-license]: https://img.shields.io/badge/license-MIT-blue.svg
