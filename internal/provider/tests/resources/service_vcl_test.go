package resources

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/integralist/terraform-provider-fastly-framework/internal/provider"
)

func TestAccResourceServiceVCL(t *testing.T) {
	serviceName := fmt.Sprintf("tf-test-%s", acctest.RandString(10))
	domain1Name := fmt.Sprintf("%s-tpff-1.integralist.co.uk", serviceName)
	domain1CommentAdded := "a random updated comment"
	domain2Name := fmt.Sprintf("%s-tpff-2.integralist.co.uk", serviceName)
	domain2NameUpdated := fmt.Sprintf("%s-tpff-2-updated.integralist.co.uk", serviceName)

	// Create a service with two domains.
	// Also set `force_destroy = false`.
	configCreate := fmt.Sprintf(`
    resource "fastly_service_vcl" "test" {
      name = "%s"
      force_destroy = false

      domains = {
        "example-1" = {
          name = "%s"
        },
        "example-2" = {
          name = "%s"
        },
      }
    }
    `, serviceName, domain1Name, domain2Name)

	// Update the first domain's comment + second domain's name (force_destroy = true).
	// We also change the order of the domains so the second is now first.
	//
	// This should result in two domains being "modified":
	//    - tpff-1 has a comment added
	//    - tpff-2 has changed name
	//
	// The domain ordering shouldn't matter (and is what we want to validate).
	//
	// IMPORTANT: Must set `force_destroy` to `true` so we can delete the service.
	configUpdate := fmt.Sprintf(`
    resource "fastly_service_vcl" "test" {
      name = "%s"
      force_destroy = true

      domains = {
        "example-2" = {
          name = "%s"
        },
        "example-1" = {
          name = "%s"
          comment = "%s"
        },
      }
    }
    `, serviceName, domain2NameUpdated, domain1Name, domain1CommentAdded)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { provider.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: configCreate,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "activate", "true"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "comment", "Managed by Terraform"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "default_ttl", "3600"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "domains.%", "2"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "domains.example-1.name", domain1Name),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "domains.example-2.name", domain2Name),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "force_destroy", "false"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "stale_if_error", "false"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "stale_if_error_ttl", "43200"),
					resource.TestCheckNoResourceAttr("fastly_service_vcl.test", "domains.example-1.comment"),
					resource.TestCheckNoResourceAttr("fastly_service_vcl.test", "domains.example-2.comment"),
				),
			},
			// Update and Read testing
			{
				Config: configUpdate,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "force_destroy", "true"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "domains.example-1.comment", domain1CommentAdded),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "domains.example-2.name", domain2NameUpdated),
					resource.TestCheckNoResourceAttr("fastly_service_vcl.test", "domains.example-2.comment"),
				),
			},
			// ImportState testing
			//
			// IMPORTANT: Import verification must happen last.
			//
			// This is because it relies on prior test cases to create resources and
			// to have a state file populated. This is so it can extract the ID needed
			// to import the resource we specify.
			//
			// Also, the `force_destroy` attribute is set by a user in their config.
			// If we had the import test before the 'update' test (where we set
			// `force` to `true`), then we would have used the last known state value
			// of `force_destroy = false` which would have prevented deleting the
			// service in time for the import test to execute.
			//
			// NOTE: We have to ignore certain resources when importing.
			//
			// Fields not part of the API response will not be set when importing.
			//
			// So this means fields like `force_destroy` have to be ignored because
			// the import test compares the import state to the state data from the
			// previous test where we had set `force_destroy = true`.
			//
			// A field like `activate` isn't set by the resource's Read() flow as we
			// define it in the schema with a default value of `true`. So because the
			// prior test config doesn't set it explicitly, at the time of the import
			// test run we don't have that value set in the current state file because
			// the default is only persisted to state after a plan/apply, and so the
			// import test would fail suggesting that we're missing the attribute.
			//
			// The `reuse` field doesn't need to be ignored as it is optional and has
			// no default value so essentially is null unless set explicitly by the
			// user in their configuration.
			//
			// The `domains` block uses the `MapNestedAttribute` type and so the map
			// keys are arbitrarily chosen by a user in their config. This means when
			// importing a service we have to generate a UUID for the key. This
			// generated key won't match with the `example-<number>` key we've used in
			// the earlier test config (see above). So to validate the import we use
			// `ImportStateCheck` to manually validate the resources exist.
			{
				ResourceName:            "fastly_service_vcl.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"activate", "domain", "force_destroy"},
				ImportStateCheck: func(is []*terraform.InstanceState) error {
					for _, s := range is {
						if numDomains, ok := s.Attributes["domains.%"]; ok {
							if numDomains != "2" {
								return fmt.Errorf("import failed: unexpected number of domains found: got %s, want 2", numDomains)
							}
						}
					}
					return nil
				},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}
