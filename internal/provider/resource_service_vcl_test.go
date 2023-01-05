package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccResourceServiceVCL(t *testing.T) {
	serviceName := fmt.Sprintf("tf-test-%s", acctest.RandString(10))
	domainName := serviceName

	// Create a service with two domains (force = false).
	configCreate := fmt.Sprintf(`
    resource "fastly_service_vcl" "test" {
      name = "%s"
      force = %t

      domains = [
        {
          name = "%s-tpff-1.integralist.co.uk"
        },
        {
          name = "%s-tpff-2.integralist.co.uk"
        },
      ]
    }
    `, serviceName, false, domainName, domainName)

	// Update the first domain's comment + second domain's name (force = true).
	// We also change the order of the domains so the second is now first.
	configUpdate := fmt.Sprintf(`
    resource "fastly_service_vcl" "test" {
      name = "%s"
      force = %t

      domains = [
        {
          name = "%s-tpff-2-updated.integralist.co.uk"
        },
        {
          comment = "a random updated comment"
          name = "%s-tpff-1.integralist.co.uk"
        },
      ]
    }
    `, serviceName, true, domainName+"-updated", domainName+"-updated")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: configCreate,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "force", "false"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "domains.#", "2"),
				),
			},
			// Update and Read testing
			{
				// IMPORTANT: Must set `force` to `true` so we can delete service.
				Config: configUpdate,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "comment", "Managed by Terraform"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "force", "true"),
				),
			},
			// ImportState testing
			//
			// IMPORTANT: Import verification must happen last.
			// This is because the `force` attribute is determined by user config and
			// can't be imported. If we had this test before the 'update' test where
			// we set `force` to `true`, then we'd use the last known state of
			// `false` and that would prevent the delete operation from succeeding.
			{
				ResourceName:            "fastly_service_vcl.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"activate", "force", "reuse"},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}
