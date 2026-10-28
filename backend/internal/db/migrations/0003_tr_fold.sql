-- tr_fold lower-cases text the Turkish way (I → ı, İ → i), independent of the
-- database locale: only ASCII and Turkish letters are folded. Text is NFC
-- normalized first, so a decomposed İ (I + combining dot) folds like İ.
CREATE FUNCTION tr_fold(s text) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    RETURN lower(translate(normalize(s, NFC), 'IİÇĞÖŞÜ', 'ıiçğöşü') COLLATE "C");
