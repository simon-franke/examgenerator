# examgen

Create a new LaTeX exam from a reference exam PDF and course slides. examgen
sends images of the reference exam to a vision model and stores the slide text
in Postgres with pgvector so the model can look it up while drafting questions.

## Requirements

- Go 1.27.3+
- [Poppler](https://poppler.freedesktop.org/) (`pdftotext`, `pdftoppm`)
- PostgreSQL with pgvector
- An OpenAI-compatible model endpoint that supports images and tool calls

On macOS, install Poppler with `brew install poppler`.

## Setup

```sh
cp .env.example .env
docker compose up -d
make build
```

Set the model and its credentials in `.env`. examgen creates the database
schema on first use.

## Usage

```sh
# Index one or more slide decks.
./bin/examgen ingest --slides week1.pdf,week2.pdf --source algorithms

# Or index every PDF in a directory.
./bin/examgen ingest --dir input/

# Create LaTeX from a reference exam.
./bin/examgen generate --exam old-exam.pdf --out output/new-exam.tex
```

Run `./bin/examgen <command> -h` to see a command's flags. Pass `--debug
output/generation.log` to `generate` to save the model and tool trace.

## Development

```sh
make test
```

By default, examgen writes LaTeX source only. Discord delivery also compiles a PDF.

## Private Discord delivery

1. Create an application and bot in the [Discord Developer Portal](https://discord.com/developers/applications).
   Install the bot in a private server you belong to. Allow direct messages from
   server members in Discord's privacy settings. No message-content intent or
   administrator permission is needed for this outbound-only integration.
2. Enable Developer Mode in Discord, right-click your profile, and copy your user ID.
3. Set `EXAMGEN_DISCORD_BOT_TOKEN` and `EXAMGEN_DISCORD_USER_ID` in your local `.env`.
   Use the bot token, never your personal account token. Keep `.env` out of Git.
4. Start Docker and prepare the PDF compiler once:

   ```sh
   docker pull --platform linux/amd64 ghcr.io/xu-cheng/texlive-full:latest
   ```

   This is a large [TeX Live image](https://github.com/xu-cheng/latex-docker/pkgs/container/texlive-full). Apple Silicon uses Docker's amd64 emulation. Compilation does not pull images automatically.

Generate and deliver to your private DMs:

```sh
./bin/examgen generate --exam input/old-exam.pdf --out output/new-exam.tex --discord
```

Add `--include-source` to attach the `.tex` file as well. Set
`EXAMGEN_DISCORD_ENABLED=true` to enable delivery for subsequent generation runs;
`--discord=false` overrides that setting. The recipient is fixed by configuration
and is never chosen by the model. The current generator creates question papers;
solution generation is not implemented.

The PDF is compiled twice using `pdflatex`, with shell escape disabled, in a
resource-limited Docker container without network access. Only a fresh temporary
folder containing the generated source is mounted; the project and credentials
are not mounted. Documents requiring external images, network access, or other
TeX engines may need manual adjustment. Compilation verifies that a PDF can be
built, not that its questions or layout are correct.

Successful runs send the PDF with a completion message. Failures after delivery configuration and compiler availability
are checked attempt a generic DM notification; detailed errors stay
in the terminal. Cancellation does not send a notification. Files already saved
remain local, and delivery failures return a nonzero exit status. If DMs are
blocked, check privacy settings and the bot's access to your shared server.

Resend existing files without calling the model or compiling again:

```sh
./bin/examgen deliver --pdf output/new-exam.pdf
./bin/examgen deliver --pdf output/new-exam.pdf --source output/new-exam.tex
```

Uploads have a conservative 10 MiB total budget; Discord may impose a lower limit.
Rate limits and ambiguous network failures are reported without automatically
retrying a send, to avoid duplicate DMs. Check your DMs before manually retrying.


See [CONTRIBUTING.md](CONTRIBUTING.md) for the required checks and pull-request workflow.
