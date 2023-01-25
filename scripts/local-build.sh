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
      # Terraform uses the following pattern to identify a provider:
      # terraform-provider-<NAME>
      #
      # When specifying a provider as a dependency the 'source' must contain the
      # username and the provider name:
      # source = "<USERNAME>/<PROVIDER_NAME>"
      #
      # This aligns with the Terraform registry URL:
      # https://registry.terraform.io/providers/<USERNAME>/<PROVIDER_NAME>
      #
      # Our provider name is 'fastly-framework'.
      # So the Terraform CLI looks for a binary ending with 'fastly-framework'.
      # We compile a binary named `terraform-provider-fastly-framework`.
      # Hence Terraform CLI knows to use that binary.
      source = "integralist/fastly-framework"
    }
  }
}
EOF
