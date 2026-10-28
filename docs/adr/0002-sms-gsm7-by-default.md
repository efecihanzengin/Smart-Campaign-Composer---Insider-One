# SMS Content is written in GSM-7 by default, with Turkish letters transliterated

A single ş, ğ, ı, İ, Ş or Ğ switches an SMS from GSM-7 (160 septets) to UCS-2 (70 characters), so "SMS ≤ 160 characters" is wrong for Turkish text. The validator is encoding-aware (extension-table characters cost 2 septets) and requires an SMS to fit in one segment of whatever encoding it ends up in. Claude is instructed to write SMS in GSM-7, transliterating ş→s, ğ→g, ı→i, İ→I, Ş→S, Ğ→G (ç, ö, ü are already in GSM-7). That is common practice in Turkish SMS marketing, and it keeps the full 160. Personalization values rendered into a GSM-7 SMS are transliterated the same way, so a Recipient named "Şükrü" can never flip their message to UCS-2 and overflow it.

## Considered Options

- **Count raw characters ≤ 160**: the case's literal wording, but it would let a 140-character Turkish SMS be split into 3 parts.
- **Let Claude pick the encoding freely**: correct, but Turkish copy gets squeezed into 70 characters most of the time.
- **Turkish National Language Shift Table (3GPP 23.038)**: keeps Turkish letters at about 155 septets, but provider support varies and it is hard to explain and test.
