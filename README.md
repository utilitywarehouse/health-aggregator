Health Aggregator
==========

A service aggregating health endpoint information from our kubernetes cluster.

[![MIT licensed][shield-license]](#)

Table of Contents
-----------------

  * [Requirements](#requirements)
  * [Usage](#usage)
  * [Endpoints](#endpoints)
    * [GET /namespaces](#get-namespaces)
    * [GET /namespaces/{namespace}/services](#get-namespaces-namespace-services)
    * [GET /services](#get-services)
    * [GET /namespaces/{namespace}/services/{service}/checks](#get-namespaces-namespace-services-service-checks)
    * [GET /namespaces/{namespace}/services/checks](#get-namespaces-namespace-services-checks)
    * [POST /namespaces](#post-reload)
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

Build and install: 

```sh
go build
go install
```

Set up the path to your local kubeconfig file (need cluster-wide read only permissions), e.g.: 

```sh
export KUBECONFIG_FILEPATH=$HOME/.kube/config
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

```
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

```
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

```
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

```
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

```
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
