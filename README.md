# What is Daisy?
Daisy is a solution for running multi-step workflows on GCE.

[![GoDoc](https://godoc.org/github.com/GoogleCloudPlatform/compute-daisy?status.svg)](https://godoc.org/github.com/GoogleCloudPlatform/compute-daisy)

The current Daisy stepset includes support for creating/deleting GCE resources,
waiting for signals from GCE VMs, streaming GCE VM logs, uploading local files
to GCE and GCE VMs, and more.

For example, Daisy is used to create Google Official Guest OS images. The
workflow:
1. Creates a Debian 8 disk and another empty disk.
2. Creates and boots a VM with the two disks.
3. Runs and waits for a script on the VM.
4. Creates an image from the previously empty disk.
5. Automatically cleans up the VM and disks.

Other use-case examples:
* Workflows for importing external physical or virtual disks to GCE.
* GCE environment deployment.
* Ad hoc GCE testing environment deployment and test running.

