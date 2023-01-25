## 0.2.0 (Unreleased)

<!--
BREAKING CHANGES:
NOTES:
FEATURES:
ENHANCEMENTS:
BUG FIXES:
DOCUMENTATION:
-->

## 0.1.0 (Month Date, Year)

BREAKING CHANGES:

> **NOTE:** HashiCorp recommends migrating ['blocks'](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/blocks) to ['nested attributes'](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/attributes#nested-attributes).

- `fastly_service_vcl`: 
  - `active_version` renamed to `last_active`
  - `cloned_version` renamed to `version`
  - `domain` renamed to `domains` and is now a nested attribute of a 'map' type.
