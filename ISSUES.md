# ISSUES

The latest [HashiCorp Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework) presents challenges that we should investigate and find more elegant solutions for.

## CRUD boundaries

Terraform doesn't have a concept of 'nested' resources, and so to workaround that design constraint our CRUD methods start to blur their boundary lines.

- *CREATE:* Runs once for top-level and nested resources.
- *READ:* Runs every time for top-level and nested resources.
- *UPDATE:* Runs every time for top-level and nested resource + CREATE/DELETE for nested resources.
- *DELETE:* Runs every time for top-level resources.

> To see the genesis of this design from the perspective of the _original_ provider, read [this](https://github.com/fastly/terraform-provider-fastly/blob/main/DEVELOPMENT.md).

## Handling errors with nested attributes.

With the original Fastly Terraform provider we had [this issue](https://github.com/fastly/terraform-provider-fastly/issues/631) related to the design of the provider using set 'blocks' to represent a nested resource (even though Terraform has no concept of a nested resource and expects a resource to be a 1:1 mapping with a single API endpoint).

This issue, although not tested, is still expected to affect the new Terraform Plugin Framework. We need to investigate, and consider whether we want to fix the issue. Because a simple `terraform refresh` appears to resolve the issue in most cases.

## Struct embedding for resource models

When defining a resource you must define an associated model using the `struct` data type.

The `fastly_service_vcl` and `fastly_service_compute` resources are almost identical but it doesn't appear to be possible to marshal the Terraform data into a model struct that uses an embedded struct, which we would want to do so that we don't have to redefine all the common fields between both resource types.

I've raised a discussion on the HashiCorp forum but have had no responses:
https://discuss.hashicorp.com/t/how-to-use-embedded-struct-for-plan-get/48841

## Identifying changes within a resource

The original Fastly Terraform provider, which used the Terraform v2 SDK, was able to identify changes in a resource using an API provided by the Terraform v2 SDK (e.g. `HasChanges`). This API does not exist in the new Terraform Plugin Framework.

This was because Terraform presumed applying a 'plan' wholesale via its API was best. In practice, not all APIs work like this and instead some require modified fields.

The result of this is that a resource now needs to identify changes itself but anything other than simple field comparisons are tricky. An example of this within our provider is identifying changes to domains. The Fastly API does not return an 'ID' for a domain, and so unless we use the correct type (we do now, we use `map` where we previously used a `set`) we have to generate one ourselves as a computed attribute and use that to track changes.

I raised a dicussion on the HashiCorp forum but the responses didn't reveal anything useful:
https://discuss.hashicorp.com/t/how-to-compare-changes-for-nested-block-with-new-framework/48333/11

HashiCorp developer @bflad has opened an issue to suggest adding back the original API:
https://github.com/hashicorp/terraform-plugin-framework/issues/526

## Generic implementations

The current implementation for identifying changes to resources is not generic. Meaning, we need a unique function for calculating changes for each resource.

Go's generics being too restrictive and the Terraform Plugin Framework leaning more towards structs and explicit types (where as the previous Terraform v2 SDK used maps and reflection more liberally) has made implementing a generic solution harder.

If we were to marshal a model struct into a map, we still need to know the underlying Terraform types so that we could type assert back to them to utilise their associated APIs, and because Terraform assigns these types to struct fields, it makes reasoning about the map keys comparison harder as well.

Some resources, such as domains, _originally_ needed custom logic (like calculating a hash of a field for a computed attribute) to execute as part of the change inspection, and the fields used for comparison are typically unique as well (e.g. with a domain, we would use the dynamically computed hash as our comparison). This makes a generic solution difficult. I've since moved the `domain` resource from a set to a map and so if all of our nested resources are a 'map' it might make implementing a generic solution easier.

Basically, it might still be possible but it requires consideration. For reference, here is the original provider's diffing logic:
https://github.com/fastly/terraform-provider-fastly/blob/d714f62c458cfd0425decc0dca3aa96297fc6063/fastly/diff.go

## Unexpected diffs

When running `terraform plan` the Terraform SDK calculates a changeset based on the configuration defined by the user and the underlying state.

The original Fastly Terraform provider had lots of issues raised related to unexpected diffs. This was primarily caused by our use of a 'set' data type for most of our resources. A set is an unordered list of items. The set calculates a hash of its entire contents and it uses this as the mechanism for identifying changes within the set.

Any change, regardless of size, will cause a new set hash. A Terraform plan would show the set as deleted and a new set created. Within the provider itself, the set isn't deleted but a single field updated.

To avoid unexpected diffs we should look to move away from using a set wherever possible.

As an example, the `domain` nested attribute is now a [`MapNestedAttribute`](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/attributes#mapnestedattribute) type and avoids the diff issue that sets introduce.
