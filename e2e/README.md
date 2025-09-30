### Tagger e2e tests

This directory contains a series of end to end tests designed to test tagger
basic features. These tests leverage [kuttl](https://kuttl.dev/) tool. In order
to run these tests you have to have tagger already deployed in a cluster that
you have access to.

If needed you can deploy a test cluster using [kind](https://kind.sigs.k8s.io/)
by running `make create-test-cluster` in the root of the repository.
