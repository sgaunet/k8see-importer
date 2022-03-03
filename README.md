
# k8see (Kubernetes Events Exporter)

Kubernetes Events Exporter is a suit of three tools to export kubernertes events in an external database. The goal is to get events in an SQL DB to be able to analyze what happened.

The 3 tools are :

* [k8see-exporter](https://github.com/sgaunet/k8see-exporter) : Deployment inside the kubernetes cluster to export events in a redis stream
* [k8see-importer](https://github.com/sgaunet/k8see-importer) : Tool that read the redis stream to import events in a database (PostGreSQL)
* [k8see-webui](https://github.com/sgaunet/k8see-webui) : Web interface to query the database
* [k8see-deploy](https://github.com/sgaunet/k8see-deploy) : kubernetes manifests to deploy k8see-exporter and also the whole procedure to deploy a full test environment in a k8s cluster (with kind).

**It's actually in development, not production ready**

# k8see-importer

The image is on docker hub : sgaunet/k8see-importer:**version**

A docker-compose in tst folder give an example to run the docker image. A binary release will be created soon.
This tool need an access to a postgresql database (the user should have access to the DB and have the rights to create tables). Database migration is handled by [dbmate](https://github.com/amacneil/dbmate) tool.


# Tests

Launch postgresql :

```
cd tst
docker-compose up -d
```

[The repository k8see-deploy](https://github.com/sgaunet/k8see-deploy) contains the whole procedure to deploy a full test environment in a k8s cluster (with kind).


## dbmate (just a reminder)

```
dbmate new       # generate a new migration file
dbmate up        # create the database (if it does not already exist) and run any pending migrations
dbmate create    # create the database
dbmate drop      # drop the database
dbmate migrate   # run any pending migrations
dbmate rollback  # roll back the most recent migration
dbmate down      # alias for rollback
dbmate status    # show the status of all migrations (supports --exit-code and --quiet)
dbmate dump      # write the database schema.sql file
dbmate wait      # wait for the database server to become available
```


