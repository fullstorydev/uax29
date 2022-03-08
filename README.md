This package tokenizes words, sentences and graphemes, based on [Unicode text segmentation](https://unicode.org/reports/tr29/#Word_Boundaries) (UAX 29), for Unicode version 13.0.0.

This is a fork off of github.com/clipperhouse/uax29/words. Modifcations have been made to the `words` package:
- A max token length can be passed in. Tokens will be split upon hitting this limit.
- Separators will be marked, so they can be omitted from the token stream if desired.
