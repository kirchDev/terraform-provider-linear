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
  # security settings — Linear moved them off the top level. Each value is the
  # minimum role the setting requires, never a boolean: "user" leaves it to
  # every workspace member, "admin" restricts it to admins.
  security_settings_json = jsonencode({
    invitationsRole        = "user"
    teamCreationRole       = "admin"
    labelManagementRole    = "user"
    templateManagementRole = "user"
    personalApiKeysRole    = "admin"
  })
}
