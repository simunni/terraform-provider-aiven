package aiven

import (
	"fmt"
	"github.com/aiven/aiven-go-client"
	"github.com/hashicorp/terraform/helper/acctest"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
	"log"
	"testing"
)

func TestAccAivenAccountTeam_basic(t *testing.T) {
	t.Parallel()

	resourceName := "aiven_account_team.foo"
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckAivenAccountTeamResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccAccountTeamResource(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAivenAccountTeamAttributes("data.aiven_account_team.team"),
					resource.TestCheckResourceAttr(resourceName, "name", fmt.Sprintf("test-acc-team-%s", rName)),
				),
			},
		},
	})
}

func testAccAccountTeamResource(name string) string {
	return fmt.Sprintf(`
		resource "aiven_account" "foo" {
			name = "test-acc-ac-%s"
		}

		resource "aiven_account_team" "foo" {
			account_id = aiven_account.foo.account_id
			name = "test-acc-team-%s"
		}

		data "aiven_account_team" "team" {
  			name = aiven_account_team.foo.name
  			account_id = aiven_account_team.foo.account_id
		}
		`, name, name)
}

func testAccCheckAivenAccountTeamResourceDestroy(s *terraform.State) error {
	c := testAccProvider.Meta().(*aiven.Client)

	// loop through the resources in state, verifying each account team is destroyed
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "aiven_account_team" {
			continue
		}

		accountId, teamId := splitResourceID2(rs.Primary.ID)

		r, err := c.Accounts.List()
		if err != nil {
			if err.(aiven.Error).Status != 404 {
				return err
			}

			return nil
		}

		for _, acc := range r.Accounts {
			if acc.Id == accountId {
				rl, err := c.AccountTeams.List(accountId)
				if err != nil {
					if err.(aiven.Error).Status != 404 {
						return err
					}

					return nil
				}

				for _, team := range rl.Teams {
					if team.Id == teamId {
						return fmt.Errorf("account team (%s) still exists", rs.Primary.ID)
					}
				}
			}
		}

	}

	return nil
}

func testAccCheckAivenAccountTeamAttributes(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		r := s.RootModule().Resources[n]
		a := r.Primary.Attributes

		log.Printf("[DEBUG] account team attributes %v", a)

		if a["account_id"] == "" {
			return fmt.Errorf("expected to get an account id from Aiven")
		}

		if a["team_id"] == "" {
			return fmt.Errorf("expected to get a team_id from Aiven")
		}

		if a["name"] == "" {
			return fmt.Errorf("expected to get a name from Aiven")
		}

		if a["create_time"] == "" {
			return fmt.Errorf("expected to get a create_time from Aiven")
		}

		if a["update_time"] == "" {
			return fmt.Errorf("expected to get a update_time from Aiven")
		}

		return nil
	}
}
