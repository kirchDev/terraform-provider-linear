# Import by the custom view's UUID, not the preferences' own: the read path goes
# through the view. The preferences themselves come back on the first plan.
terraform import linear_view_preferences.example a1b2c3d4-0000-0000-0000-000000000000
