package provider

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccResourceServiceVCL(t *testing.T) {
	serviceName := fmt.Sprintf("tf-test-%s", acctest.RandString(10))
	domainName := serviceName

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccResourceServiceVCLConfig(serviceName, domainName, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "force", "false"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "domain.#", "2"),
				),
			},
			// Update and Read testing
			{
				// IMPORTANT: Must set `force` to `true` so we can delete service.
				Config: testAccResourceServiceVCLConfig(serviceName+"-updated", domainName+"-updated", true),
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

func testAccResourceServiceVCLConfig(serviceName, domainName string, force bool) string {
	domainName1 := domainName
	domainName2 := domainName
	needle := "-updated"

	// NOTE: We only want to update the first domain.
	// So we strip the needle from the second domain.
	if strings.Contains(domainName, needle) {
		domainName2 = strings.ReplaceAll(domainName2, needle, "")
	}

	return fmt.Sprintf(`
resource "fastly_service_vcl" "test" {
  name = "%s"
  force = %t

  domain {
    name = "%s-tpff-1.integralist.co.uk"
  }

  domain {
    name = "%s-tpff-2.integralist.co.uk"
  }
}
`, serviceName, force, domainName1, domainName2)
}
