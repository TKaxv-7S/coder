ALTER TABLE workspace_agent_scripts ADD COLUMN dependencies jsonb NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN workspace_agent_scripts.dependencies IS 'Declared ordering dependencies for this script, as a JSON array of {resource_address, required_status} objects derived from coder_script_order resources.';
