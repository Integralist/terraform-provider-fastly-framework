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
func TestAccResourceServiceVCLStandardBehaviours(t *testing.T) {
	serviceName := fmt.Sprintf("tf-test-%s", acctest.RandString(10))
	domain1Name := fmt.Sprintf("%s-tpff-1.integralist.co.uk", serviceName)
	domain1CommentAdded := "an added comment"
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
			//
			// Terraform's import testing sees that `last_active` was set previously
			// but after importing it's no longer set. That's because we only set
			// `last_active` if `activate=true` and during an import the computed
			// attribute value (nor its default) is available. So we explicitly add
			// it to the ImportStateVerifyIgnore list.
			{
				ResourceName:            "fastly_service_vcl.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"activate", "domain", "force_destroy", "last_active"},
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
			// Delete testing automatically occurs at the end of the TestCase.
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
			// Delete testing automatically occurs at the end of the TestCase.
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

	apiClient := fastly.NewAPIClient(fastly.NewConfiguration())
	ctx := fastly.NewAPIKeyContextFromEnv(helpers.APIKeyEnv)

	var computeServiceID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { provider.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			deleteReq := apiClient.ServiceAPI.DeleteService(ctx, computeServiceID)
			_, _, err := deleteReq.Execute()
			if err != nil {
				return fmt.Errorf("failed to delete service '%s' outside of Terraform: %w", computeServiceID, err)
			}
			return nil
		},
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
					req := apiClient.ServiceAPI.CreateService(ctx)
					resp, _, err := req.Name(fmt.Sprintf("tf-test-compute-service-%s", acctest.RandString(10))).ResourceType("wasm").Execute()
					if err != nil {
						return "", fmt.Errorf("failed to create Compute service: %w", err)
					}
					computeServiceID = *resp.ID
					return computeServiceID, nil
				},
				ExpectError: regexp.MustCompile(`Expected service type vcl, got: wasm`),
			},
			// Delete testing automatically occurs at the end of the TestCase.
			// But when creating a resource outside of TF we have to manually delete.
			// See CheckDestroy() function above.
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
			// Delete testing automatically occurs at the end of the TestCase.
		},
	})
}

// The following test validates three state drift scenarios.
//
// The first is that when we have more than one service version, where the
// latter version is not 'active' (because the user has set `activate=false`),
// that we return and track that version instead of any prior active version.
//
// i.e. we're allowing for `version` attribute to drift from `last_active`.
//
// The second scenario is when we create a service with `activate=false`, so we
// have a non-active version 1. We then make an update which triggers the
// service version to be cloned (resulting in a non-active version 2). Because
// there is no active service version, we'll track service version 2 while
// last_active will be null as there is no prior active service version.
//
// The third scenario is when the user has multiple active service versions but
// they need to manually revert the service version via the UI and so the next
// time they run the Terraform provider they want it to be tracking the correct
// service version.
func TestAccResourceServiceVCLStateDrift(t *testing.T) {
	serviceName := fmt.Sprintf("tf-test-%s", acctest.RandString(10))
	domain1Name := fmt.Sprintf("%s-tpff-1.integralist.co.uk", serviceName)
	domain1CommentAdded := "an added comment"
	domain1CommentUpdated := "an updated comment"
	domain2Name := fmt.Sprintf("%s-tpff-2.integralist.co.uk", serviceName)

	configCreate1 := configServiceVCLCreate(configServiceVCLCreateOpts{
		activate:     true,
		forceDestroy: true,
		serviceName:  serviceName,
		domain1Name:  domain1Name,
		domain2Name:  domain2Name,
	})

	configCreate2 := configServiceVCLCreate(configServiceVCLCreateOpts{
		activate:     false,
		forceDestroy: true,
		serviceName:  serviceName,
		domain1Name:  domain1Name,
		domain2Name:  domain2Name,
	})

	// Update the first domain by adding a comment + set `activate=false`.
	// This will result in a new service version that's inactive.
	// We want Terraform to be tracking this version and not the active version.
	configUpdate1 := fmt.Sprintf(`
    resource "fastly_service_vcl" "test" {
      activate = false
      force_destroy = true
      name = "%s"

      domains = {
        "example-1" = {
          name = "%s"
          comment = "%s"
        },
        "example-2" = {
          name = "%s"
        },
      }
    }
    `, serviceName, domain1Name, domain1CommentAdded, domain2Name)

	// Update the first domain's comment.
	// This will result in a new service version that's inactive.
	// We want Terraform to be tracking this version and not the prior inactive version.
	configUpdate2 := fmt.Sprintf(`
    resource "fastly_service_vcl" "test" {
      activate = false
      force_destroy = true
      name = "%s"

      domains = {
        "example-1" = {
          name = "%s"
          comment = "%s"
        },
        "example-2" = {
          name = "%s"
        },
      }
    }
    `, serviceName, domain1Name, domain1CommentUpdated, domain2Name)

	// Update the first domain by adding a comment.
	// This will result in a new service version that's active.
	// We'll then cause a side-effect that re-activates version 1.
	// We want Terraform to track version=1 and last_active=2.
	configUpdate3 := fmt.Sprintf(`
    resource "fastly_service_vcl" "test" {
      activate = true
      force_destroy = true
      name = "%s"

      domains = {
        "example-1" = {
          name = "%s"
          comment = "%s"
        },
        "example-2" = {
          name = "%s"
        },
      }
    }
    `, serviceName, domain1Name, domain1CommentAdded, domain2Name)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { provider.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: configCreate1,
			},
			// Update and Read testing
			{
				Config: configUpdate1,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "last_active", "1"), // we expect `version` to drift from `last_active`
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "version", "2"),
				),
			},
			// ImportState testing
			//
			// NOTE: This test case validates the default import behaviour.
			// Which is to import the most recent active service version.
			// i.e. There was a recently active service version (1) so we use it.
			// Otherwise we would've ended up tracking the latest service version (2).
			// The next import test will validate the latter scenario.
			{
				ResourceName:      "fastly_service_vcl.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Terraform's import testing sees that `last_active` was set previously
				// but after importing it's no longer set. That's because we only set
				// `last_active` if `activate=true` and during an import the computed
				// attribute value (nor its default) is available. So we explicitly add
				// it to the ImportStateVerifyIgnore list.
				//
				// Similarly, in the last test step the `version` was set to `2` and so
				// that's what the import test will try to validate will be the value
				// after an import completes. But in this particular scenario, when
				// we're importing we will attempt to track the more recent 'active'
				// service version (which is version 1) and so we explicitly add
				// `version` to the ImportStateVerifyIgnore list and instead use
				// `ImportStateCheck` to validate the value is `1`.
				ImportStateVerifyIgnore: []string{"activate", "domain", "force_destroy", "last_active", "version"},
				ImportStateCheck: func(is []*terraform.InstanceState) error {
					for _, s := range is {
						if version, ok := s.Attributes["version"]; ok && version != "1" {
							return fmt.Errorf("import failed: expected service version 1 (the last active): got %s", version)
						}
					}
					return nil
				},
			},
			// Delete resource by emptying the TF config
			{
				Config: `# can't use an empty string`,
			},
			// Create and Read testing
			{
				Config: configCreate2,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("fastly_service_vcl.test", "last_active"), // expect `last_active` to be null
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "version", "1"),
				),
			},
			// Update and Read testing
			{
				Config: configUpdate2,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("fastly_service_vcl.test", "last_active"), // expect `last_active` to be null
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "version", "2"),
				),
			},
			// ImportState testing
			//
			// NOTE: This test case validates the 'no active version' behaviour.
			// Which is we'll track the latest service version, not the last active.
			// i.e. If `activate=false` then `last_active` is never set.
			//
			// Terraform's import test behaviour is to compare the imported state to
			// the previous state, so as the last step test found `version` to be 2,
			// this means we expect the imported state to match because the latest
			// service version is 2 and that's what the import logic selected as there
			// was no prior active service version.
			{
				ResourceName:            "fastly_service_vcl.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"activate", "domain", "force_destroy"},
			},
			// Delete resource by emptying the TF config
			{
				Config: `# can't use an empty string`,
			},
			// Create and Read testing
			{
				Config: configCreate1,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "last_active", "1"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "version", "1"),
				),
			},
			// Update and Read testing
			{
				Config: configUpdate3,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "last_active", "2"),
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "version", "2"),
				),
			},
			// Trigger side-effect of activating version 1 outside of Terraform.
			// We use the same config as previous TestStep (so no config changes).
			{
				Config: configUpdate3,
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						if r, ok := s.RootModule().Resources["fastly_service_vcl.test"]; ok {
							if id, ok := r.Primary.Attributes["id"]; ok {
								apiClient := fastly.NewAPIClient(fastly.NewConfiguration())
								ctx := fastly.NewAPIKeyContextFromEnv(helpers.APIKeyEnv)
								version := int32(1)
								clientReq := apiClient.VersionAPI.ActivateServiceVersion(ctx, id, version)
								_, httpResp, err := clientReq.Execute()
								if err != nil {
									return fmt.Errorf("failed to activate service outside of Terraform: %w", err)
								}
								defer httpResp.Body.Close()
							}
						}
						return nil
					},
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "last_active", "2"), // expect no change as no Read() flow has executed yet but active service is now 1 outside of Terraform
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "version", "2"),     // expect no change as no Read() flow has executed yet but active service is now 1 outside of Terraform
				),
				ExpectNonEmptyPlan: true, // TF config has a comment value for domain 1 while the refreshed state after Read() will pull version 1 of the service that has no comment
			},
			// RefreshState testing
			//
			// NOTE: This test case validates the previous test step.
			// We can now see that the state file sees version 1 is the active version.
			{
				ResourceName: "fastly_service_vcl.test",
				RefreshState: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "last_active", "1"), // we can see the side-effect of Service Version 1 being reactivated
					resource.TestCheckResourceAttr("fastly_service_vcl.test", "version", "1"),     // we can see the side-effect of Service Version 1 being reactivated
				),
				ExpectNonEmptyPlan: true, // TF config has a comment value for domain 1 while the refreshed state after Read() will pull version 1 of the service that has no comment
			},
			// Delete testing automatically occurs at the end of the TestCase.
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
