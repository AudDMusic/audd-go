`scanFiles` is an example of an interaction with the AudD API.

## What it does

Basically, it recognized all the music in a folder, writes it to audd.csv file and sets the ID3 tags (artists, titles, album covers, etc.)

## Is it useful?

Maybe. A couple of users asked as for this utility, so we made it.

## Why is the enteprise endpoint used?

You can actually use this utility for hours-long mixes. But the reason was to avoid asking for installing ffpmeg and to provide a bit better accuracy.

## Can I do the same in other languages?

Sure! For example, take a look at [this Rust music tagger](https://github.com/octowaddle/mtag).
