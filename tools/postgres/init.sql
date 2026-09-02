-- Bootstrap the local database.
--
-- The application role must NOT be a superuser and must NOT have BYPASSRLS,
-- otherwise the row level security policies created in the migrations are
-- silently ignored (see docs/adr/0007-rls-multitenancy.md).
CREATE ROLE ciftpay WITH LOGIN PASSWORD 'ciftpay' NOSUPERUSER NOCREATEDB NOCREATEROLE;
CREATE DATABASE ciftpay OWNER ciftpay;
