-- Add 'bedrock-mantle' to ai_provider_type. It selects the AWS Bedrock mantle
-- wire protocol (bedrock-mantle.{region}.api.aws/anthropic/v1/messages), as
-- opposed to the legacy InvokeModel protocol carried by 'bedrock'. The aibridge
-- runtime serves both through the Anthropic client; the enum just needs the
-- discriminator so DB-driven providers can carry it. No migration casts existing
-- rows to this value, so ADD VALUE is safe in the single migration transaction.
-- Mirrors the precedent in 000506_ai_provider_type_copilot_value.up.sql.
ALTER TYPE ai_provider_type ADD VALUE IF NOT EXISTS 'bedrock-mantle';
