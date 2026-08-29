-- Hash all plaintext emails (emails containing '@') using SHA256 (hex encoded)
UPDATE users
SET email = encode(sha256(email::bytea), 'hex')
WHERE email LIKE '%@%';
