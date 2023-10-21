# Colocation Groups

Imagine you have an application with three components `A`, `B`, `C`. If
you want to colocate `A` and `B` you have to write the following config:

```toml
[serviceweaver]
binary = "./app"

colocate = [["A", "B"]]
```

At runtime, the unit of deployment is a colocation group (either specified
explicitly in the config as shown above, or computed by the deployer (`group C`)).

When running these groups they can have different requirements. For example, in
case of an ML serving application, the models run by `group(A, B)` require a
"weight sharing" compression algorithm while the model run by `group(C)` requires
a "Huffman coding" compression algorithm. Similarly, `group(A, B)` may require
routing based on GPU because the running models are GPU intensive, while
`group(C)` requires routing based on CPU - in this case we may want to specify a
different load metric. Some of the knobs are more deployer specific; e.g.,
readiness/liveness probes for kubernetes deployments, autoscalers for distributed
deployments.

## Problem Statement

Each colocation group can have multiple knobs to control their behavior at
runtime, and some knobs are more general, while the others are deployer specific.

**Q**: What's the right approach to specify these knobs such that:

* the config is readable
* the config doesn't look overly complicated
* hard to misconfigure things
* it's easy to understand where to add new knobs
* give each deployer only configs that it really needs

## Options

### Option 1

Knobs values are set when the colocation group is defined.

```toml
[serviceweaver]
binary = "./app"

[groups.foo]
components = ["A", "B"]
identity = "..."
secrets = "..."
volume = "..."
resources = "..."
replicas = "..."
load_metric = "..."
sharding_algo = "..."
compression_algo = "..."
probes = "..."
autoscaler = "..."

[multi]
# nothing for group `foo`

[kube]
# nothing for group `foo`
```

**Pros**:
* all knobs for a given colocation group in a single place;
* harder to misconfigure things because there is a single place to override the
value of a knob;
* don't have to think much where to add a new knob.

**Cons**:
* (extremely) challenging to set knob values that make sense for all the deployers.
E.g., for the `multi` deployer the `volume` knob points to a local file, directory; for
kubernetes the `volume` knob can also point to `NFS`, `GCEPersistentDisk`,
`AWSElasticBlockStore`, `ConfigMap`, etc;
* all knobs exposed to all deployers even if they don't need them - e.g., `multi`
doesn't need `probe`, `autoscaler`, `identity` while the `kube` deployer doesn't
need `replicas`;
* can lead to misconfigurations; e.g., `replicas` sets the number of replicas,
but `autoscaler` also has options to set min/max replicas, although one is intended
to be used by `multi` and the other by `kube`;

### Option 2

Knobs values are set in the deployers.

```toml
[serviceweaver]
binary = "./app"

[groups.foo]
components = ["A", "B"]

[multi]
[multi.groups.foo]
secrets = "base64_encoded_string or secret_file_name"
replicas = "num_replicas_to_run"
volume = "path_local_directory or path_local_file"
load_metric = "requests_per_sec"
sharding_algo = "hash_sharding"
compression_algo = "weight_sharing_algo"

[kube]
[kube.groups.foo]
identity = "service_account_name"
secrets = {"secret_name", "mount_path", "read_only"}
volume = "host_path or nfs_server_path or volume_claim_name"
resources = {"cpu_reqs", "mem_reqs", "cpu_limits", "mem_limits"}
probes = {"knobs_for_liveness_probe", "knobs_for_readiness_probe"}
autoscaler = {"cpu threshold", "mem threshold", "min/max replicas"}
load_metric = "requests_per_sec"
sharding_algo = "hash_sharding"
compression_algo = "weight_sharing_algo"
```

Note that for this option, we can also inline the group definition in the per
deployer section.

**Pros**:
* adding a new knob is easier because we don't have to think what's an implementation
that makes sense across all deployers; e.g., `volume`, `secrets`;
* each deployer only sees knobs that make sense for it; e.g., the `multi` deployer
doesn't see the `probes` knob;

**Cons**:
* same knobs can be present in multiple places in the config (under the assumption
that there is a single toml file shared across deployers); e.g., both `multi` and
`kube` have knobs like `secrets`, `volume`;
* some knobs are defined in multiple places with the same values; e.g., `load_metric`,
`sharding_algo`, `compression_algo`;
* config looks more complicated.


### Option 3

Knobs values are split between group definition and the deployers.

```toml
[serviceweaver]
binary = "./app"

[groups.foo]
components = ["A", "B"]
load_metric = "requests_per_sec"
sharding_algo = "hash_sharding"
compression_algo = "weight_sharing_algo"

[multi]
[multi.groups.foo]
secrets = "base64_encoded_string or secret_file_name"
replicas = "num_replicas_to_run"
volume = "path_local_directory or path_local_file"

[kube]
[kube.groups.foo]
identity = "service_account_name"
secrets = {"secret_name", "mount_path", "read_only"}
volume = "host_path or nfs_server_path or volume_claim_name"
resources = {"cpu_reqs", "mem_reqs", "cpu_limits", "mem_limits"}
probes = {"knobs_for_liveness_probe", "knobs_for_readiness_probe"}
autoscaler = {"cpu threshold", "mem threshold", "min/max replicas"}
```

**Pros**:
* knobs that make sense for all deployers in the app config; knobs that are
deployer specific in the per deployer sections; avoids specifying knobs with same
values across all deployer (e.g., `load_metric` in Option 2);
* each deployer only sees knobs that make sense for it;
* harder to misconfigure knobs; e.g., if I want to change the `sharding_algo`, I
can just do that only in one place.

**Cons**:
* creates more tension when adding a new knob - should it be in the app config or
in the per deployer config? E.g., for some knobs like `probes` it's obvious but
for others like `secrets`it's unclear (`env` is another one where we have a generic
way to do it, but kubernetes also allow you to export them from a config map).

### Decision

TBD

I like Option 3 the most, because it forces us to keep knobs that are general
enough in the app config, while the ones that are very deployer specific in the
per deployer section. However, the challenge is to decide when certain knobs
should be in one category or the other.
