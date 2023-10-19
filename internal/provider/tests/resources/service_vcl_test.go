package resources

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

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
			// This is because the `force_destroy` attribute is determined by user
			// config and can't be imported. If we had this test before the 'update'
			// test where we set `force` to `true`, then we'd use the last known state
			// of `false` and that would prevent the delete operation from succeeding.
			//
			// NOTE: We have to ignore the domains attribute when importing because
			// of data type used (MapNestedAttribute). The map keys are arbitrarily
			// chosen by a user in their config and so when importing a service we
			// have to generate a uuid for the key, which doesn't match with the
			// `example-<number>` key we've used in the earlier test config (above).
			{
				ResourceName:            "fastly_service_vcl.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"activate", "domains", "force_destroy", "reuse"},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}
