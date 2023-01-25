#!/bin/bash
#
# REFERENCE:
# https://developer.hashicorp.com/terraform/tutorials/providers-plugin-framework/providers-plugin-framework-provider#prepare-terraform-for-local-provider-install

cat <<EOF >"$HOME/.terraformrc"
provider_installation {
  dev_overrides {
      "integralist/fastly-framework" = "$GOPATH/bin"
  }
}
EOF

echo ""
echo "A development overrides file has been generated at $HOME/.terraformrc"
echo "Either cd into the ./examples/ directory in this repo or create your own configuration."
echo "Terraform config without a provider version will use the locally built version of the provider."

cat <<"EOF"

EXAMPLE:

terraform {
  required_providers {
    fastly = {
      # The compiled binary name is `terraform-provider-fastly-framework`.
      # The name has the format `terraform-provider-<NAME>`
      # So the 'fastly-framework' in the source below aligns with this pattern.
      source = "integralist/fastly-framework"
    }
  }
}
EOF
