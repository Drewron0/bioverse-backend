-- +goose Up
ALTER TABLE donations 
    ADD COLUMN IF NOT EXISTS target_pet_id UUID REFERENCES virtual_pets(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS coral_nickname VARCHAR(100),
    ADD COLUMN IF NOT EXISTS include_bio_shield BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS payment_url TEXT;

-- +goose Down
ALTER TABLE donations 
    DROP COLUMN IF EXISTS target_pet_id,
    DROP COLUMN IF EXISTS coral_nickname,
    DROP COLUMN IF EXISTS include_bio_shield,
    DROP COLUMN IF EXISTS payment_url;
