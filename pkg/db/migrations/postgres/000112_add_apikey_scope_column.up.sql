CREATE TYPE backend.apikey_scope AS ENUM ('puzzle', 'portal');

ALTER TABLE backend.apikeys ADD COLUMN scope backend.apikey_scope NOT NULL;

UPDATE backend.apikeys SET scope = 'puzzle'::backend.apikey_scope;
