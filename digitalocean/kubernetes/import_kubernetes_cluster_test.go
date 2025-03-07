package kubernetes_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/acceptance"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/config"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/kubernetes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

var (
	clusterStateIgnore = []string{
		"kube_config",                      // because kube_config was completely different for imported state
		"node_pool.0.node_count",           // because import test failed before DO had started the node in pool
		"updated_at",                       // because removing default tag updates the resource outside of Terraform
		"registry_integration",             // registry_integration state can not be known via the API
		"destroy_all_associated_resources", // destroy_all_associated_resources state can not be known via the API
	}
)

func TestAccDigitalOceanKubernetesCluster_ImportBasic(t *testing.T) {
	clusterName := acceptance.RandomTestName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { acceptance.TestAccPreCheck(t) },
		ProviderFactories: acceptance.TestAccProviderFactories,
		CheckDestroy:      testAccCheckDigitalOceanKubernetesClusterDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDigitalOceanKubernetesConfigBasic(testClusterVersionLatest, clusterName),
				// Remove the default node pool tag so that the import code which infers
				// the need to add the tag gets triggered.
				Check: testAccDigitalOceanKubernetesRemoveDefaultNodePoolTag(clusterName),
			},
			{
				ResourceName:            "digitalocean_kubernetes_cluster.foobar",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: clusterStateIgnore,
			},
		},
	})
}

func TestAccDigitalOceanKubernetesCluster_ImportErrorNonDefaultNodePool(t *testing.T) {
	testName1 := acceptance.RandomTestName()
	testName2 := acceptance.RandomTestName()

	config := fmt.Sprintf(testAccDigitalOceanKubernetesCusterWithMultipleNodePools, testClusterVersionLatest, testName1, testName2)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { acceptance.TestAccPreCheck(t) },
		ProviderFactories: acceptance.TestAccProviderFactories,
		CheckDestroy:      testAccCheckDigitalOceanKubernetesClusterDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				// Remove the default node pool tag before importing in order to
				// trigger the multiple node pool import error.
				Check: testAccDigitalOceanKubernetesRemoveDefaultNodePoolTag(testName1),
			},
			{
				ResourceName:      "digitalocean_kubernetes_cluster.foobar",
				ImportState:       true,
				ImportStateVerify: false,
				ExpectError:       regexp.MustCompile(kubernetes.MultipleNodePoolImportError.Error()),
			},
		},
	})
}

func TestAccDigitalOceanKubernetesCluster_ImportNonDefaultNodePool(t *testing.T) {
	testName1 := acceptance.RandomTestName()
	testName2 := acceptance.RandomTestName()

	config := fmt.Sprintf(testAccDigitalOceanKubernetesCusterWithMultipleNodePools, testClusterVersionLatest, testName1, testName2)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { acceptance.TestAccPreCheck(t) },
		ProviderFactories: acceptance.TestAccProviderFactories,
		CheckDestroy:      testAccCheckDigitalOceanKubernetesClusterDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
			},
			{
				ResourceName:            "digitalocean_kubernetes_cluster.foobar",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: clusterStateIgnore,
			},
			// Import the non-default node pool as a separate digitalocean_kubernetes_node_pool resource.
			{
				ResourceName:            "digitalocean_kubernetes_node_pool.barfoo",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: clusterStateIgnore,
			},
		},
	})
}

func testAccDigitalOceanKubernetesRemoveDefaultNodePoolTag(clusterName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client := acceptance.TestAccProvider.Meta().(*config.CombinedConfig).GodoClient()

		clusters, resp, err := client.Kubernetes.List(context.Background(), &godo.ListOptions{})
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				return fmt.Errorf("No clusters found")
			}

			return fmt.Errorf("Error listing Kubernetes clusters: %s", err)
		}

		var cluster *godo.KubernetesCluster
		for _, c := range clusters {
			if c.Name == clusterName {
				cluster = c
				break
			}
		}
		if cluster == nil {
			return fmt.Errorf("Unable to find Kubernetes cluster with name: %s", clusterName)
		}

		for _, nodePool := range cluster.NodePools {
			tags := make([]string, 0)
			for _, tag := range nodePool.Tags {
				if tag != kubernetes.DigitaloceanKubernetesDefaultNodePoolTag {
					tags = append(tags, tag)
				}
			}

			if len(tags) != len(nodePool.Tags) {
				nodePoolUpdateRequest := &godo.KubernetesNodePoolUpdateRequest{
					Tags: tags,
				}

				_, _, err := client.Kubernetes.UpdateNodePool(context.Background(), cluster.ID, nodePool.ID, nodePoolUpdateRequest)
				if err != nil {
					return err
				}
			}
		}

		return nil
	}
}

const testAccDigitalOceanKubernetesCusterWithMultipleNodePools = `%s

resource "digitalocean_kubernetes_cluster" "foobar" {
  name    = "%s"
  region  = "lon1"
  version = data.digitalocean_kubernetes_versions.test.latest_version

  node_pool {
    name       = "default"
    size       = "s-1vcpu-2gb"
    node_count = 1
  }
}

resource "digitalocean_kubernetes_node_pool" "barfoo" {
  cluster_id = digitalocean_kubernetes_cluster.foobar.id
  name       = "%s"
  size       = "s-1vcpu-2gb"
  node_count = 1
}
`
