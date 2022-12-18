package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccServiceVCLResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccServiceVCLResourceConfig(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "force", "true"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "id", "example-id"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "domain.#", "1"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "fastly_service_vcl.test",
				ImportState:       true,
				ImportStateVerify: true,
				// This is not normally necessary, but is here because this
				// example code does not have an actual upstream service.
				// Once the Read method is able to refresh information from
				// the upstream service, this can be removed.
				ImportStateVerifyIgnore: []string{"comment", "domain", "force", "name"},
			},
			// Update and Read testing
			{
				Config: testAccServiceVCLResourceConfig(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "comment", "Managed by Terraform"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "force", "false"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccServiceVCLResourceConfig(force bool) string {
	name := fmt.Sprintf("tf-test-%s", acctest.RandString(10))

	return fmt.Sprintf(`
resource "fastly_service_vcl" "test" {
  name = "%s"
  force = %t

  domain {
    name = "%s.example.com"
  }
}
`, name, force, name)
}
