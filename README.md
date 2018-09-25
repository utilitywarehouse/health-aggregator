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
  * [POST /reload](#post-reload)
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

#### Step 1 - Annotate your namespace and services

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

#### Step 2 - Include your namespace

Add the namespace name to the `RESTRICT_NAMESPACE` environment variable in the `health-aggregator` kubernetes manifest in the `health-aggregator` namespace for your environment.

For example:

```yaml
- name: RESTRICT_NAMESPACE
  value: smartmetering,partner-portal,energy,crm,customer-platform,jtc,customer-onboarding,insurance
```

#### Step 3 - Reload

Now that you've added annotations, force a reload. See here: [POST /reload](#post-reload).

### To add an instance of health-aggregator to your namespace

Note: you require an instance of mongo running in your cluster.

Follow `Step 1 - Annotate your namespace and services`.

Then, copy the manifest from the health-aggregator namespace and modify the following:

* The namespace name
* Set `RESTRICT_NAMESPACE` to your own namespace name
* Set the Ingress host as required for your instance

Apply the manifest and run `Step 3 - Reload` as above.

To expose checks via the API you need health-aggregator-api. For the GUI, run an instance of health-aggregator-ui.

## GUI

A UI exists for health aggregator ([health-aggregator-ui](https://github.com/utilitywarehouse/health-aggregator-ui)) and this can be found here:

  `https://health-aggregator.{dev|prod}.uw.systems/?ns={namespace_name}`

E.g:

[https://health-aggregator.prod.uw.systems/?ns=partner-portal](https://health-aggregator.prod.uw.systems/?ns=partner-portal)

## Endpoints

The application [health-aggregator-api](https://github.com/utilitywarehouse/health-aggregator-api) exposes namespace and service configuration that health-aggregator knows about, as well as health check results.

### POST /reload

This POST with empty body carries out the discovery process for all health endpoints once more, allowing any annotation changes or new services and namespaces to be picked up.

Changes to deployments for services which health-aggregator knows about are automatically picked up.

Reloads can be triggered from the health aggregator ui here:

* [https://health-aggregator.dev.uw.systems/admin](https://health-aggregator.dev.uw.systems/admin)
* [https://health-aggregator.prod.uw.systems/admin](https://health-aggregator.prod.uw.systems/admin)

## License

Health Aggregator is licensed under the [MIT](https://github.com/utilitywarehouse/health-aggregator/blob/master/LICENSE) license.

[golang]: https://golang.org/
[docker]: https://www.docker.com/
[shield-license]: https://img.shields.io/badge/license-MIT-blue.svg
