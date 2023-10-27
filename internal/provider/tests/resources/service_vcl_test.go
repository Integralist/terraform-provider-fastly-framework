package resources

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/fastly/fastly-go/fastly"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider"
)

// The following test validates the standard service behaviours.
// e.g. creating/updating the resource and nested resources.
func TestAccResourceServiceVCL(t *testing.T) {
	serviceName := fmt.Sprintf("tf-test-%s", acctest.RandString(10))
	domain1Name := fmt.Sprintf("%s-tpff-1.integralist.co.uk", serviceName)
	domain1CommentAdded := "a random updated comment"
	domain2Name := fmt.Sprintf("%s-tpff-2.integralist.co.uk", serviceName)
	domain2NameUpdated := fmt.Sprintf("%s-tpff-2-updated.integralist.co.uk", serviceName)

	configCreate := configServiceVCLCreate(configServiceVCLCreateOpts{
		activate:     true,
		forceDestroy: false,
		serviceName:  serviceName,
		domain1Name:  domain1Name,
		domain2Name:  domain2Name,
	})

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

	resource.ParallelTest(t, resource.TestCase{
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

// The following test validates the service deleted_at behaviour.
// i.e. if deleted_at is not empty, then remove the service resource.
func TestAccResourceServiceVCLDeletedAtCheck(t *testing.T) {
	serviceName := fmt.Sprintf("tf-test-%s", acctest.RandString(10))
	domain1Name := fmt.Sprintf("%s-tpff-1.integralist.co.uk", serviceName)
	domain2Name := fmt.Sprintf("%s-tpff-2.integralist.co.uk", serviceName)

	configCreate := configServiceVCLCreate(configServiceVCLCreateOpts{
		activate:     true,
		forceDestroy: true,
		serviceName:  serviceName,
		domain1Name:  domain1Name,
		domain2Name:  domain2Name,
	})

	resource.ParallelTest(t, resource.TestCase{
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
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "force_destroy", "true"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "stale_if_error", "false"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "stale_if_error_ttl", "43200"),
					resource.TestCheckNoResourceAttr("fastly_service_vcl.test", "domains.example-1.comment"),
					resource.TestCheckNoResourceAttr("fastly_service_vcl.test", "domains.example-2.comment"),
				),
			},
			// Trigger side-effect of deleting resource outside of Terraform.
			// We use the same config as previous TestStep (so no config changes).
			//
			// Because Terraform executes a refresh/plan after each test case, we
			// validate that the final plan is not empty using `ExpectNonEmptyPlan`.
			{
				Config: configCreate,
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						if r, ok := s.RootModule().Resources["fastly_service_vcl.test"]; ok {
							if id, ok := r.Primary.Attributes["id"]; ok {
								apiClient := fastly.NewAPIClient(fastly.NewConfiguration())
								ctx := fastly.NewAPIKeyContextFromEnv(helpers.APIKeyEnv)
								version := int32(1)
								deactivateReq := apiClient.VersionAPI.DeactivateServiceVersion(ctx, id, version)
								_, httpResp, err := deactivateReq.Execute()
								if err != nil {
									return fmt.Errorf("failed to deactivate service outside of Terraform: %w", err)
								}
								defer httpResp.Body.Close()
								deleteReq := apiClient.ServiceAPI.DeleteService(ctx, id)
								_, httpResp, err = deleteReq.Execute()
								if err != nil {
									return fmt.Errorf("failed to delete service outside of Terraform: %w", err)
								}
								defer httpResp.Body.Close()
							}
						}
						return nil
					},
				),
				ExpectNonEmptyPlan: true, // We expect a diff for creating our service.
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

// The following test validates the service type import behaviour.
// i.e. when importing a service, check the service type matches the resource.
// e.g. importing a Compute service ID into a VCL service resource.
func TestAccResourceServiceVCLImportServiceTypeCheck(t *testing.T) {
	serviceName := fmt.Sprintf("tf-test-%s", acctest.RandString(10))
	domain1Name := fmt.Sprintf("%s-tpff-1.integralist.co.uk", serviceName)
	domain2Name := fmt.Sprintf("%s-tpff-2.integralist.co.uk", serviceName)

	configCreate := configServiceVCLCreate(configServiceVCLCreateOpts{
		activate:     false,
		forceDestroy: true,
		serviceName:  serviceName,
		domain1Name:  domain1Name,
		domain2Name:  domain2Name,
	})

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { provider.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// We need a resource to be created by Terraform so we can import into it.
			{
				Config: configCreate,
			},
			// ImportState testing
			{
				ResourceName: "fastly_service_vcl.test",
				ImportState:  true,
				ImportStateIdFunc: func(_ *terraform.State) (string, error) {
					apiClient := fastly.NewAPIClient(fastly.NewConfiguration())
					ctx := fastly.NewAPIKeyContextFromEnv(helpers.APIKeyEnv)
					req := apiClient.ServiceAPI.CreateService(ctx)
					resp, _, err := req.Name(fmt.Sprintf("tf-test-compute-service-%s", acctest.RandString(10))).ResourceType("wasm").Execute()
					if err != nil {
						return "", fmt.Errorf("failed to create Compute service: %w", err)
					}
					return *resp.ID, nil
				},
				ExpectError: regexp.MustCompile(`Expected service type vcl, got: wasm`),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

// The following test validates importing a specific service version.
// e.g. terraform import fastly_service_vcl.test xxxxxxxxxxxxxxxxxxxx@2.
func TestAccResourceServiceVCLImportServiceVersion(t *testing.T) {
	serviceName := fmt.Sprintf("tf-test-%s", acctest.RandString(10))
	domain1Name := fmt.Sprintf("%s-tpff-1.integralist.co.uk", serviceName)
	domain2Name := fmt.Sprintf("%s-tpff-2.integralist.co.uk", serviceName)

	configCreate := configServiceVCLCreate(configServiceVCLCreateOpts{
		activate:     true,
		forceDestroy: true,
		serviceName:  serviceName,
		domain1Name:  domain1Name,
		domain2Name:  domain2Name,
	})

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { provider.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// We need a resource to be created by Terraform so we can import into it.
			{
				Config: configCreate,
			},
			// Clone the service version and return the import ID to use.
			{
				ResourceName: "fastly_service_vcl.test",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					if r, ok := s.RootModule().Resources["fastly_service_vcl.test"]; ok {
						if id, ok := r.Primary.Attributes["id"]; ok {
							apiClient := fastly.NewAPIClient(fastly.NewConfiguration())
							ctx := fastly.NewAPIKeyContextFromEnv(helpers.APIKeyEnv)
							req := apiClient.VersionAPI.CloneServiceVersion(ctx, id, 1)
							_, _, err := req.Execute()
							if err != nil {
								return "", fmt.Errorf("failed to clone service version: %w", err)
							}
							return id + "@2", nil
						}
					}
					return "", nil
				},
				ImportStateCheck: func(is []*terraform.InstanceState) error {
					for _, s := range is {
						serviceVersion := s.Attributes["version"]
						if serviceVersion != "2" {
							return fmt.Errorf("import failed: unexpected service version found: got %s, want 2", serviceVersion)
						}
					}
					return nil
				},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

type configServiceVCLCreateOpts struct {
	activate, forceDestroy                bool
	serviceName, domain1Name, domain2Name string
}

// configServiceVCLCreate returns a TF config that consists of a VCL service
// with two domains + configurable `activate` and `force_destroy` attributes.
//
// NOTE: We use this config for a lot of the tests.
// But occasionally we need to tweak the force_destroy attribute.
func configServiceVCLCreate(opts configServiceVCLCreateOpts) string {
	return fmt.Sprintf(`
    resource "fastly_service_vcl" "test" {
      activate = %t
      force_destroy = %t
      name = "%s"

      domains = {
        "example-1" = {
          name = "%s"
        },
        "example-2" = {
          name = "%s"
        },
      }
    }
  `, opts.activate, opts.forceDestroy, opts.serviceName, opts.domain1Name, opts.domain2Name)
}
