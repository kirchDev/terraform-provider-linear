# The workspace singleton: manage-not-create. Applying it adopts the workspace
# the API key belongs to; destroying it only drops the resource from state.
resource "linear_workspace_settings" "this" {
  name              = "kirchDev"
  git_branch_format = "{teamKey}/{issueIdentifier}-{issueTitle}"

  # 1 = Monday … 5 = Friday
  working_days            = [1, 2, 3, 4, 5]
  fiscal_year_start_month = 0

  roadmap_enabled   = true
  customers_enabled = true

  # Member invitation, team creation and label management restrictions live in
  # security settings — Linear moved them off the top level.
  security_settings_json = jsonencode({
    allowMembersToInvite            = false
    restrictTeamCreationToAdmins    = true
    restrictLabelManagementToAdmins = true
  })
}
