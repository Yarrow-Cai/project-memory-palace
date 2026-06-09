DEFAULT_CONFIG = """\
schema_version: 1
project_name: ""
default_recall_limit: 5
search:
  use_embeddings: false
  include_rejected_by_default: false
"""

DEFAULT_AGENT_RULES = """\
schema_version: 1
must_remember_after:
  - solved_problem
  - implemented_feature
  - fixed_bug
  - produced_design
  - clarified_project_goal
  - discovered_module_workflow
  - changed_previous_decision
  - established_convention
do_not_remember:
  - temporary_command_output
  - low_value_debug_noise
  - unconfirmed_guess
  - duplicate_without_new_information
  - secrets_tokens_accounts_private_data
memory_value_checks:
  - helps_future_project_understanding
  - reduces_repeated_analysis
  - has_clear_source
  - not_duplicate_or_links_to_existing_memory
  - supersedes_old_memory_when_needed
"""
