---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "fastly_example Data Source - terraform-provider-fastly-framework"
subcategory: ""
description: |-
  Example data source
---

# fastly_example (Data Source)

Example data source

## Example Usage

```terraform
data "fastly_example" "example" {
  configurable_attribute = "some-value"
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Optional

- `configurable_attribute` (String) Example configurable attribute

### Read-Only

- `id` (String) Example identifier

