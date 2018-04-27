Health Aggregator
==========

A service aggregating health endpoint information from our kubernetes cluster.

[![MIT licensed][shield-license]](https://github.com/utilitywarehouse/health-aggregator/blob/master/LICENSE)

Table of Contents
-----------------

* [Requirements](#requirements)
* [Usage](#usage)
* [Endpoints](#endpoints)
  * [GET /namespaces](#get-namespaces)
  * [GET /namespaces/{namespace}/services](#get-namespacesnamespaceservices)
  * [GET /services](#get-services)
  * [GET /namespaces/{namespace}/services/{service}/checks](#get-namespacesnamespaceservicesservicechecks)
  * [GET /namespaces/{namespace}/services/checks](#get-namespacesnamespaceserviceschecks)
  * [POST /reload](#post-reload)
* [License](#license)

Requirements
------------

Health Aggregator requires the following to run:

* [Golang][golang] 1.9+
* [Docker][docker]

Usage
-----

**Running Locally**
From the root directory, go get all dependencies:

```sh
go get ./...
```

Build, test and install:

```sh
make all
```

Set up the path to your local kubeconfig file (need cluster-wide read only permissions), e.g.:

```sh
export KUBECONFIG_FILEPATH=$HOME/.kube/config
```

Note that http calls to service health checks cannot be carried out when outside the cluster, so the responses are faked when the app detects that it is running out of cluster.

Other optional params:

```
health-aggregator --help
```

```
      --port                       Port to listen on (env $PORT) (default "8080")
      --ops-port                   The HTTP ops port (env $OPS_PORT) (default 8081)
      --kubeconfig                 (optional) absolute path to the kubeconfig file (env $KUBECONFIG_FILEPATH) (default "")
      --log-level                  Log level (e.g. INFO, DEBUG, WARN) (env $LOG_LEVEL) (default "INFO")
      --mongo-connection-string    Connection string to connect to mongo ex mongodb:27017/ (env $MONGO_CONNECTION_STRING) (default "127.0.0.1:27017/")
      --mongo-drop-db              Set to true in order to drop the DB on startup (env $MONGO_DROP_DB)
      --delete-checks-after-days   Age of check results in days after which they are deleted (env $DELETE_CHECKS_AFTER_DAYS) (default 7)
```

Start MongoDB:

```sh
docker-compose up -d
```

Start the app:

```sh
health-aggregator
```

Endpoints
-----

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
    ...
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
    ...
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
    ...
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
  ...
  ]
```

### GET /namespaces/{namespace}/services/checks

Returns a list of the most recent check responses for each of the services in the specified namespace.

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
  ...
  ]
```

### POST /reload

This POST with empty body carries out the discovery process for all health endpoints once more, allowing any annotation changes or new services and namespaces to be picked up.

License
-------

Health Aggregator is licensed under the [MIT](https://github.com/utilitywarehouse/health-aggregator/blob/master/LICENSE) license.

[golang]: https://golang.org/
[docker]: https://www.docker.com/
[shield-license]: https://img.shields.io/badge/license-MIT-blue.svg
