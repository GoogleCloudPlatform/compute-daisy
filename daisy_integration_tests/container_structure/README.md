Container structure tests validate the Docker image that is consumed at gcr.io/compute-image-tools/daisy. They are
executed by [container-structure-test](https://github.com/GoogleContainerTools/container-structure-test).

**Running locally**

First install [container-structure-test](https://github.com/GoogleContainerTools/container-structure-test).

Example execution:

```shell
container-structure-test test \
  --image gcr.io/compute-image-tools/daisy \
  --config base.yaml
```
