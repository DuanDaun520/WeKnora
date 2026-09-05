-- 000100 down: drop usage metering tables. Data loss by design (ledger +
-- prices); only run when decommissioning the billing feature.
DROP INDEX IF EXISTS idx_usage_records_tenant_time;
DROP INDEX IF EXISTS idx_usage_records_user_time;
DROP INDEX IF EXISTS idx_usage_records_time;
DROP TABLE IF EXISTS model_usage_records;
DROP TABLE IF EXISTS model_prices;
