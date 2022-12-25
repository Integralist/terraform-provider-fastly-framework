package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccResourceServiceVCL(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandString(10))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccResourceServiceVCLConfig(name, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "force", "false"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "domain.#", "1"),
				),
			},
			// Update and Read testing
			{
				// IMPORTANT: Must set `force` to `true` so we can delete service.
				Config: testAccResourceServiceVCLConfig(name, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "comment", "Managed by Terraform"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "force", "true"),
				),
			},
			// ImportState testing
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

func testAccResourceServiceVCLConfig(name string, force bool) string {
	return fmt.Sprintf(`
resource "fastly_service_vcl" "test" {
  name = "%s"
  force = %t

  domain {
    name = "%s-terraform-provider-fastly-framework.integralist.co.uk"
  }
}
`, name, force, name)
}
