# dots v0.2.0 仕様設計書

## 文書情報

- 対象バージョン: v0.2.0
- 前提: v0.1.0 仕様（docs/develop/v0.1/architecture.md）。この文書に書いていないことは v0.1.0 の仕様に従う。
- 実装言語: Go 1.26 以上（v0.1.0 の 1.24 から上げる。12.4）
- 設定ファイル: dots.toml
- 元になった草案: GitHub Issue #1「v1.0までの実装草案」の「v0.2.0 — 初回セットアップと普段使い」
- 関連Issue: #6（設定から消したルールのリンクが残る）、#9（apply で変更したファイルの source を表示する）、#11（競合の確認でチェックボックスへ移れない）、#4（先行入力で確認プロンプトが止まる）

## 1. 目的と範囲

v0.1.0 で、dots.toml に書いたルールどおりにリンクを作り、診断し、バックアップから戻すところまでできた。v0.2.0 では、その前後にある手作業を減らす。

- 新しいPCで、dotfilesリポジトリの取得から apply の直前までの準備をまとめて済ませる。
- 既存の設定ファイルを dots の管理に移す。
- dots の管理をやめるとき、作ったリンクを安全に外す。
- 設定から消したルールのリンクを見つけて片付ける。
- どこにいてもリポジトリへ移動できる。

### 1.1 草案Issueとの対応

| 草案のIssue | v0.2.0 での扱い | 本書 |
| --- | --- | --- |
| Issue 1: `dots bootstrap` | 含める | 8章 |
| Issue 2: Gitリポジトリからの取得 | 含める。`dots bootstrap <url>` の最初の手順として実装する | 7章 |
| Issue 3: `dots add <path>` | 含める | 9章 |
| Issue 4: `dots unlink` | 含める。指定した配置先はルールの削除または ignore への追加まで行う。Issue 13 と一緒に設計する | 10章 |
| Issue 5: `dots cd` | 含める | 11章 |
| Issue 12: 実機E2E（CI） | v0.1.0 で実装済み。新コマンドのシナリオを追加する | 14章 |
| Issue 13: 設定から消したルールのリンク | 含める。stateファイルを導入する | 5章、10章、12章 |

草案以外に、次の GitHub Issue も v0.2.0 で扱う。

| Issue | v0.2.0 での扱い | 本書 |
| --- | --- | --- |
| #9: apply で変更したファイルの source を表示する | 含める | 12.3 |
| #11: 競合の確認でチェックボックスへ移れない | 含める。huh をやめ、bubbletea v2 で確認プロンプトを書いて直す | 12.4 |
| #4: 先行入力で確認プロンプトが止まる | 確認プロンプトを bubbletea v2 で書くことで一緒に解決する | 12.4 |

Issue 2 は草案では「v0.2.0候補」だった。取得自体は git コマンドに任せ、dots は取得先の確認と結果の表示だけを持つ。実装量が小さく、bootstrap の前段として必要なので v0.2.0 に含め、bootstrap に組み込む。

### 1.2 v0.2.0 の非目標

v0.1.0 の非目標（パッケージマネージャー、環境変数管理、Nixの代替、テンプレートエンジン、プロファイル切り替え）は引き続き対象外とする。加えて、次のことは v0.2.0 でやらない。

- `dots status`、`dots diff`。Issue #1 のコメントにあるヘルプ例には含まれているが、v0.2.0 では追加しない。
- リポジトリの更新（pull）や、一部のファイルだけの取得。前者は git を直接使う。後者は v0.4.0 の source プラグインで扱う。
- Gitへの commit、push。add も init も行わない。
- unlink 時に、リンクを source の中身のコピーで置き換えること。
- 複数のdotfilesリポジトリを同時に使い分けること（5.4、15章）。
- プラグイン機構。bootstrap の外部処理呼び出しは、プラグインではなく設定に書いたコマンドを実行する（8.4）。

## 2. v0.1.0 からの変更点

| 項目 | 変更内容 | 互換性 |
| --- | --- | --- |
| リポジトリの解決 | `--repo <dir>` と `DOTS_REPO` で指定できる。dots.toml がカレントディレクトリから見つからないときは、記録したリポジトリを使う（4章） | v0.1.0 でエラーだった場面が成功するようになる。指定がなければ既存の動作は変わらない |
| リポジトリの記録 | bootstrap <url> と apply がリポジトリの場所をデータディレクトリに記録する（4.2） | 新規 |
| stateファイル | apply がリンクを記録する（5章） | 新規。v0.1.0 で作ったリンクは、v0.2.0 で最初に apply したときに記録される |
| 設定ファイル | トップレベルに `bootstrap` を追加する（8.2） | 既存の dots.toml はそのまま動く |
| doctor | stale（設定にはないが dots が作ったリンク）を表示する（12.1） | stale があると終了コード1になる。stale は v0.2.0 で記録したリンクからしか生じないので、更新直後に結果が変わることはない |
| apply | stale があると件数と片付け方を表示する（12.2）。変更した項目に `← source` を表示する（12.3） | stale を自動では消さない |
| doctor の source 表示 | `-> source` を `← source` に変える（12.3） | 表示だけの変更。doctor の出力を解析しているスクリプトには影響する |
| Go の最小バージョン | 1.24 から 1.26 に上げる（12.4） | `go install` で入れる場合は Go 1.26 以上が必要になる。配布バイナリには影響しない |
| 確認プロンプト | huh をやめ、bubbletea で書いた確認プロンプトにする（12.4） | 選択肢と答えの意味は v0.1.0 と同じ。操作方法が変わる |
| 新コマンド | bootstrap、add、unlink、cd | - |

## 3. CLIコマンド

v0.1.0 のコマンドに次を追加する。

| コマンド | 動作 |
| --- | --- |
| dots bootstrap | 前提条件の確認、ディレクトリ作成、設定した準備コマンドの実行を順に行う。 |
| dots bootstrap <url> [<dir>] | git でリポジトリを取得して記録し、続けて上の準備を行う。 |
| dots bootstrap --dry-run | 変更を行わず、各手順の予定を表示する。 |
| dots bootstrap --yes | 準備コマンドの実行確認を省略する。 |
| dots add <path> <dest> | 既存のファイルまたはディレクトリをリポジトリ内の `<dest>` へ移し、必要ならルールを追加し、元の場所にリンクを作る。 |
| dots add <path> <dest> --dry-run | 移動、dots.toml の書き換え、作るリンクを表示する。 |
| dots unlink <target>... | 指定した配置先のリンクを外し、dots.toml から管理を外す（[dots] はエントリー削除、[auto] は ignore に追加）。 |
| dots unlink --stale | 設定にはないが dots が作ったリンクを外す。実行前に確認する。 |
| dots unlink ... --dry-run | 外すもの、触らないもの、dots.toml の書き換えを表示する。 |
| dots cd | リポジトリルートの絶対パスを標準出力に表示する。 |

すべてのコマンドに共通のオプションとして `--repo <dir>` を加える。リポジトリルートを指定する（4.1）。短い形は設けない。init と bootstrap <url> では使えない。

`dots --help` の表示は次のとおりとする。グループ分けは Issue #1 のコメントにある例に合わせ、v0.2.0 で存在するコマンドだけを並べる。

```
$ dots --help

A brief, declarative, flexible dotfiles manager.

Usage:
  dots <command> [options]

Core:
  apply       Apply dotfiles
  doctor      Check configuration and managed files

Usability:
  init        Initialize a dotfiles repository
  bootstrap   Set up the environment for dots
  add         Add files to dots
  restore     Restore files from backups
  unlink      Unlink managed files
  edit        Open dots.toml in your editor
  cd          Print the repository path

Options:
  --repo <dir>    Use <dir> as the repository
  -h, --help      Show help
  -v, --version   Show version

Use "dots <command> --help" for more information.
```

## 4. リポジトリの解決

v0.1.0 では、doctor、apply、restore、edit はカレントディレクトリから親へ dots.toml を探し、見つからなければエラーにしていた。v0.2.0 では、どこにいても dots を使えるように、リポジトリの場所をファイルに記録し、見つからないときはそれを使う。あわせて、リポジトリを明示的に指定する方法を2つ加える。

### 4.1 解決の順序

対象コマンドは doctor、apply、restore、edit、bootstrap、add、unlink、cd とする。init は対象外で、v0.1.0 と同じくカレントディレクトリに dots.toml を作る。`<url>` を渡した bootstrap も対象外で、取得先をリポジトリとする（7章）。

1. `--repo <dir>` オプションがあれば、`<dir>` をリポジトリルートとする。`<dir>` はカレントディレクトリを基準に解決する。`<dir>` の直下に dots.toml がなければエラーにする。親ディレクトリは探さない。
2. 環境変数 `DOTS_REPO` が空でなければ、その値をリポジトリルートとする。絶対パスでなければエラーにする。直下に dots.toml がなければエラーにする。
3. カレントディレクトリから親ディレクトリへ順に dots.toml を探す（v0.1.0 と同じ）。見つかればそのディレクトリをリポジトリルートとする。
4. 見つからない場合、リポジトリの記録（4.2）を読む。記録があり、そのディレクトリに dots.toml があれば、そこをリポジトリルートとする。
5. どれでも見つからない場合はエラーにする。記録があったのに dots.toml がない場合は、そのパスをエラーに含める（例: `dots: no dots.toml in the recorded repository ~/dotfiles`）。記録もない場合は、`dots init` または `dots bootstrap <url>` を案内する。

1と2は利用者が明示した指定なので、失敗しても次の方法へは進まない。3を4より優先するのは、v0.1.0 の動作を変えないためである。

`--repo` はリポジトリを指定するだけで、カレントディレクトリは変えない。コマンドライン引数の相対パス（add の `<path>`、unlink の `<target>`、restore の `<archive>`）は、`--repo` があっても実際のカレントディレクトリを基準に解決する。ファイル名は dots.toml に固定し、設定ファイルのパスを直接指定するオプションは設けない。

結果表示の先頭にあるリポジトリルートの表示（v0.1.0 の8章）には、2または4で決めた場合だけ出所を付記する（例: `Repository: ~/dotfiles (recorded)`、`Repository: ~/dotfiles (DOTS_REPO)`）。

bootstrap の準備コマンドには `DOTS_REPO` を渡す（8.4）。準備コマンドの中で dots を呼ぶと、同じリポジトリが使われる。

### 4.2 リポジトリの記録

`dots cd` や `dots edit` をどこからでも使えるように、普段使うリポジトリルートの場所をファイルに保存する。バックアップ（v0.1.0 の7.1）と同じデータディレクトリに置く。

- Linux、FreeBSD、OpenBSDなど: 有効な XDG_DATA_HOME/dots/repository。XDG_DATA_HOME が未設定または空なら ~/.local/share/dots/repository。設定されているが絶対パスでない場合はエラーにする。
- macOS: ~/Library/Application Support/dots/repository
- Windows: %LOCALAPPDATA%\dots\repository

中身は、リポジトリルートの絶対パスを UTF-8 で1行だけ書いたテキストとする（末尾の改行は任意）。利用者が手で書き換えてもよい。空、複数行、絶対パスでない場合は、記録が壊れているとして警告を表示し、記録がないものとして扱う。書き込みは一時ファイルを経由して rename する。

記録を書くのは次の場合だけとする。

| コマンド | 書く条件 |
| --- | --- |
| `dots bootstrap <url>` | 取得に成功し（取得済みでスキップした場合を含む）、取得先の直下に dots.toml があった場合（7.2） |
| `dots apply` | `--dry-run` でなく、検証エラーなしに処理を終えた場合。部分適用で終わった場合も書く。1〜4のどの方法で決めたリポジトリでも書く |

`dots init` は記録しない。init で作ったリポジトリは、そこで最初に `dots apply` を実行したときに記録される。記録を別のリポジトリへ切り替えたいときは、そのリポジトリで `dots apply` を実行するか、記録のファイルを書き換える。

記録したリポジトリで作業するときに、`--repo` や `DOTS_REPO` を使う必要はない。この2つは、別のリポジトリを一時的に使うときや、スクリプトから場所を固定したいときに使う。

## 5. stateファイル

dots が作ったリンクを記録する。設定から消したルールのリンク（#6）を見つけるために使う。リポジトリの場所の記録（4.2）とは別のファイルである。

### 5.1 保存先

バックアップ（v0.1.0 の7.1）と同じ考え方で、OSごとに次の場所に置く。

- Linux、FreeBSD、OpenBSDなど: 有効な XDG_STATE_HOME/dots/state.json。XDG_STATE_HOME が未設定または空なら ~/.local/state/dots/state.json。設定されているが絶対パスでない場合はエラーにする。
- macOS: ~/Library/Application Support/dots/state.json
- Windows: %LOCALAPPDATA%\dots\state.json

ディレクトリとファイルは、現在のユーザーだけがアクセスできる権限で作る。

### 5.2 形式

UTF-8 の JSON とする。

```json
{
  "version": 1,
  "links": [
    {
      "target": "/home/user/.vimrc",
      "link": "dotfiles/vimrc",
      "repository": "/home/user/dotfiles",
      "source": "vimrc",
      "recorded_at": "2026-10-07T12:00:00Z"
    }
  ]
}
```

| キー | 内容 |
| --- | --- |
| version | 形式のバージョン。v0.2.0 では 1 |
| links[].target | 配置先の実行時の絶対パス |
| links[].link | 作成したシンボリックリンクのリンク文字列（Readlink が返す値） |
| links[].repository | そのリンクを作ったときのリポジトリルートの絶対パス |
| links[].source | リポジトリルートからの source 項目の相対パス（/ 区切り）。表示用 |
| links[].recorded_at | 記録したUTC日時（RFC 3339） |

パスはOSの形式で保存する。target どうしの比較は v0.1.0 の5.4と同じ正規化で行う（Windows と macOS では大文字・小文字を区別しない）。links は target の辞書順で保存する。

### 5.3 記録と削除

- apply は、作成したリンクと、すでに ValidLink だったリンクを記録する。v0.1.0 で作ったリンクも、v0.2.0 で apply すれば記録される。
- 同じ target の記録はひとつだけ持つ。新しく記録するときは上書きする。
- apply は、記録の target が存在しなくなっていれば、その記録を消す。
- unlink は、外したリンクの記録を消す（10章）。
- add は、作ったリンクを記録する（9章）。
- doctor と各コマンドの --dry-run は state を変更しない。

書き込みは、同じディレクトリの一時ファイルに書き終えてから最終名へ rename する。dots を同時に複数実行した場合の排他制御は行わない。後から書いた方の内容が残る。

### 5.4 stale の判定

stale は「現在の設定にはないが、dots が作ったリンク」である。次の条件をすべて満たす記録を stale とする。

1. 記録の repository が、今回使っているリポジトリルートと同じ。
2. 記録の target が、現在の設定から展開した配置先（上書きされたルールを含む）に含まれない。

stale の記録は、target の現在の状態で次のように分ける。判定には Lstat と Readlink を使い、リンク先はたどらない。

| 状態 | 条件 | unlink --stale での動作 |
| --- | --- | --- |
| StaleLink | target がシンボリックリンクで、リンク文字列が記録の link と一致する | リンクを削除し、記録を消す |
| StaleChanged | target がシンボリックリンクだがリンク文字列が違う、または通常ファイルやディレクトリになっている | 触らない。記録を消し、報告する |
| StaleGone | target が存在しない | 記録を消す（表示はしない） |

リンク文字列だけで比較するのは、リポジトリを移動してリンク切れになったリンクも、dots が作ったものとして扱うためである。Windows では、比較の前に両方の / を \ にそろえる。

配置先のディレクトリを走査して、リポジトリ内を指すリンクを探す方法は採らない。削除されたルールの配置先がどこにあったかは設定から分からず、走査する範囲を決められないためである。

別のリポジトリで作った記録は stale として扱わない。リポジトリを別の場所へ移動した場合、移動前の記録は stale として検出できない。v0.2.0 ではこれを制限として README に書く。

### 5.5 stateが読めない場合

- ファイルがない: 記録がないものとして扱う。stale は検出されない。エラーにも警告にもしない。
- JSON として読めない、必須キーがない: 読めなかったことを警告として表示し、記録がないものとして扱う。doctor は `! error` として報告する（終了コード1）。apply は書き込む前に、壊れたファイルを `state.json.broken-<UTC日時>` に rename して残し、新しい state を作る。退避先は表示する。unlink --stale は何も削除せずエラー終了する。
- version が 1 より大きい: 新しい dots で書かれたものとみなし、書き換えない。読み取り専用のコマンドは警告を出して記録なしとして続ける。state を書くコマンド（apply、add、unlink）は変更前にエラー終了する。

## 6. 共通の約束

この文書で追加するコマンドは、次を v0.1.0 と共通にする。

- パスの規則（v0.1.0 の4章）。設定ファイルに書くパスは / 区切りで、絶対パスを禁止する。
- 結果表示の形式と記号（v0.1.0 の8章、本書の13章）。先頭にリポジトリルートを表示する。
- 終了コード（v0.1.0 の8章）。0 は成功、1 は失敗または問題の検出、2 は使い方の誤り、130 はユーザーによる中断。
- --dry-run はファイルシステム、dots.toml、state、リポジトリの記録を変更しない。外部コマンドも実行しない。

### 6.1 dots.toml の書き換え

add と unlink は dots.toml を書き換える。コメントや並び順を残すため、TOML ライブラリで書き出し直さず、テキストとして必要な箇所だけを変える。

書き換えは次の3種類に限る。

| 操作 | 使うコマンド | 内容 |
| --- | --- | --- |
| エントリーの追加 | add | `[dots]`（または `[dots.<os>]`）に `"<target>" = "<source>"` の行を挿入する |
| エントリーの削除 | unlink | `[dots]` または `[dots.<os>]` の該当するキーと値の組を、その行ごと削除する。同じ行の末尾のコメントも消える。直前のコメント行は残す |
| ignore への追加 | unlink | `[[auto]]` または `[[auto.<os>]]` のルールの ignore 配列の末尾に要素を追加する。ignore がなければ、そのルールの最後のキーと値の組の直後に `ignore = ["<名前>"]` の行を挿入する |

- テーブルや配列の範囲は、TOML パーサーが返す位置情報で決める。行の見た目（`[` で始まる行など）では判断しない。複数行文字列や複数行の配列の途中に挿入することはない。
- エントリーの追加で、テーブル見出しがある場合は、そのテーブルに属する最後のキーと値の組の直後の行に挿入する。キーがなければ見出しの直後に挿入する。見出しがない場合は、ファイルの末尾に空行、見出し、エントリーの行を追加する。
- ignore 配列が複数行で書かれている場合は、最後の要素の次の行に、その要素と同じ字下げで追加する。1行で書かれている場合は、閉じ括弧の直前に `, "<名前>"` を追加する。
- `dots = { ... }` のようなインラインテーブル、`dots."~/.vimrc" = ...` のようなドット付きキー、1行に複数のキーと値の組を書いている場合など、上の方法で書き換えられない書き方は、変更前にエラーにする。dots.toml を手で編集するよう案内する。
- キーと値は TOML の基本文字列として必要なエスケープを行う。
- 改行コードは既存ファイルに合わせる（CRLF のファイルには CRLF で書く）。
- 書き換え後のテキストを読み込み直し、検証を通ること、読み込んだ結果が「元の設定に今回の変更だけを加えたもの」と一致することを確認してから書き込む。一致しなければ書き込まずにエラーにする。
- 書き込みは一時ファイルを経由して rename する。

## 7. dots bootstrap: リポジトリの取得

### 7.1 使い方

```
dots bootstrap <url> [<dir>]
dots bootstrap <url> [<dir>] --dry-run
```

`<url>` を渡すと、bootstrap はまずリポジトリを git で取得し、取得先をリポジトリとして記録する（4.2）。続けて、取得したリポジトリで8章の準備を行う。`<url>` を渡さない場合は、取得を行わず、4章の方法で決めたリポジトリに対して8章の準備だけを行う。

新しいPCでは、次の2つのコマンドで dotfiles を配置できる。

```sh
dots bootstrap https://github.com/user/dotfiles ~/dotfiles
dots apply
```

`dots init` は v0.1.0 と同じく、カレントディレクトリに dots.toml の雛形を作るだけで、取得もリポジトリの記録も行わない。

- `<url>` は git clone が受け付ける形式（https、ssh、scp形式の `git@host:owner/repo.git`、ローカルパスなど）をそのまま渡す。dots は形式を検証しない。
- `<dir>` は取得先。カレントディレクトリを基準に解決する。省略した場合は、`<url>` の最後のパス要素から末尾の `/` と `.git` を除いた名前を使う（git clone の既定と同じ）。名前を決められない場合はエラーにし、`<dir>` の指定を求める。
- `<url>` と `--repo` は同時に指定できない（使い方の誤り）。`<url>` があるときは `DOTS_REPO` を無視する。

### 7.2 処理の順序

1. git が PATH にあるか確認する。ない場合は変更前にエラーにし、git のインストールを案内する。
2. 取得先を確認する。
   - 存在しない、または空のディレクトリ: 取得する。
   - git リポジトリで、`git -C <dir> remote get-url origin` の結果が `<url>` と一致する: 取得済みとみなし、取得をスキップする（`✓`）。前回の bootstrap が途中で失敗した後の再実行で、もう一度取得しないためである。
   - それ以外（ファイル、シンボリックリンク、空でないディレクトリ、origin が違うリポジトリ）: 変更前にエラーにする。確認なしでの上書きは行わず、上書きするオプションも設けない。
3. `git clone -- <url> <dir>` を実行する。標準入力・標準出力・標準エラーはそのまま渡し、git の進捗表示や認証の入力を利用者が見られるようにする。
4. git が失敗した場合は、git の終了コードを含めてエラーにする（dots 自身の終了コードは1）。取得先の後片付けは git に任せ、dots は削除しない。
5. 取得先の直下に dots.toml があるか確認する。
   - ある場合: リポジトリの記録（4.2）を取得先に更新する。すでに別の記録があった場合も上書きし、以前の記録を表示する。そのまま8章の準備へ進む。
   - ない場合: 記録は更新しない。dots.toml がないこと、取得先で `dots init` を実行すれば作れることを表示し、終了コード1で終える。8章の準備には進まない。

`--dry-run` では git を実行しない。取得先が未取得の場合は dots.toml を読めないので、取得の予定だけを表示し、8章の準備は「取得後に決まる」と表示する。取得済み（2の2番目）の場合は、8章の準備の予定も表示する。

### 7.3 認証

認証は git に設定済みの仕組み（credential helper、SSH エージェントなど）に任せる。dots は認証情報を受け取らず、保存もしない。

`<url>` にユーザー情報（例: `https://user:token@host/...`）が含まれる場合、dots の表示とエラーではユーザー情報を `***` に置き換える。--dry-run で表示する git コマンドも同じとする。git にはそのまま渡す。

## 8. dots bootstrap

### 8.1 目的

新しい環境で dots apply を実行する前に必要な準備をまとめて行う。dots はパッケージマネージャーにはならない。Nix などのインストールは、利用者がリポジトリに置いたスクリプトを bootstrap から呼ぶ形で行う。

何度実行しても困らないことを前提にする。ディレクトリ作成はすでにあれば何もしない。準備コマンドは `creates` を指定すれば、2回目以降はスキップされる（8.4）。

### 8.2 設定

dots.toml のトップレベルに `bootstrap` テーブルを追加する。省略可能である。

```toml
[bootstrap]
requires = ["git"]
directories = ["~/.local/bin", "~/.cache/zsh"]

[[bootstrap.run]]
command = ["sh", "scripts/install-nix.sh"]
creates = "~/.nix-profile"

[bootstrap.darwin]
requires = ["brew"]
directories = ["~/Library/Application Support/dots-example"]

[[bootstrap.darwin.run]]
command = ["sh", "scripts/macos-defaults.sh"]

[bootstrap.windows]
directories = ["~/AppData/Local/nvim-data"]

[[bootstrap.windows.run]]
command = ["powershell", "-NoProfile", "-File", "scripts/setup.ps1"]
```

| キー | 型 | 内容 |
| --- | --- | --- |
| requires | 文字列の配列 | PATH 上に存在すべきコマンド名 |
| directories | 文字列の配列 | 作成するディレクトリ。target と同じ規則で解決する（v0.1.0 の4章） |
| run | テーブルの配列 | 実行する準備コマンド（8.4） |
| <os> | テーブル | そのOSでだけ使う requires、directories、run |

OS別の設定は、共通の設定を置き換えずに後ろへ追加する。実行中のOSのテーブルだけを使い、他のOSのテーブルは検証だけ行う。

設定は v0.1.0 と同じく厳密に解釈する。

- トップレベルで許可するキーは dots、auto、bootstrap になる。
- `bootstrap` と `bootstrap.<os>` で使えるキーは上の表のものだけとする。`bootstrap.<os>` の中に OS 名のテーブルは書けない。OS 名は v0.1.0 の5.1と同じGoの実行環境名に限る。
- requires の要素は空文字列や / または \ を含む値を設定エラーとする。
- directories の要素は、"~" と "." そのもの、絶対パス、リポジトリの外へ出る相対パスを設定エラーとする。
- 設定エラーは最初の1件で止めず、場所（例: `bootstrap.darwin.run[0]`）を付けてすべて報告する。

### 8.3 実行の順序

1. `<url>` があれば、リポジトリを取得する（7章）。
2. 設定の検証。dots.toml 全体（dots、auto を含む）を検証する。エラーがあれば何もせず終了する。
3. 前提条件の確認。次をすべて確認し、問題があればまとめて報告して変更前に終了する。
   - requires の各コマンドが PATH 上にあるか。
   - シンボリックリンクを作れるか。Windows では apply と同じ権限確認（Developer Mode または SeCreateSymbolicLinkPrivilege）を行う。他のOSでは常に成功とする。
   - バックアップと state の保存先を決められるか（XDG_DATA_HOME、XDG_STATE_HOME、LOCALAPPDATA の値が不正でないか）。
4. 計画の表示と確認。実行する準備コマンドが1つ以上ある場合だけ、計画を表示したうえで実行してよいか確認する（8.5）。
5. ディレクトリの作成。directories を共通、OS別の順に処理する。
6. 準備コマンドの実行。run を共通、OS別の順に、書いた順に実行する。
7. 結果と次の手順（`dots apply --dry-run`）を表示する。

bootstrap は apply を実行しない。

### 8.4 directories と run

directories の各要素は次のように扱う。

| 現在の状態 | 動作 |
| --- | --- |
| 存在しない | 親を含めて作成する |
| ディレクトリ | 何もしない |
| ディレクトリを指すシンボリックリンク | 何もしない |
| 通常ファイル、その他のシンボリックリンク | エラー。そこで止める |

run の各要素は次のキーを持つ。

| キー | 必須 | 内容 |
| --- | --- | --- |
| command | 必須 | 引数の配列。シェルを介さず実行する。空の配列は設定エラー |
| creates | 省略可 | このパスが存在すれば実行しない。存在の判定は Lstat で行い、リンク切れのシンボリックリンクも存在するとみなす。directories と同じ規則で解決するが、絶対パスも許可する（`/nix` のように、準備コマンドが root 権限で作る場所を確認するため）。存在を調べるだけで、dots がそこを変更することはない |

- command の最初の要素に / または \ が含まれる場合は、リポジトリルートからの相対パスとして解決する。含まれない場合は PATH から探す。2番目以降の要素は加工せず渡す。
- 作業ディレクトリはリポジトリルートとする。
- 環境変数は dots の実行環境を引き継ぎ、`DOTS_REPO`（リポジトリルートの絶対パス）と `DOTS_OS`（Goの実行環境名）を加える。
- 標準入力・標準出力・標準エラーはそのまま渡す。スクリプトが sudo のパスワードなどを尋ねられるようにするためである。
- 終了コードが0以外なら失敗とし、そこで止める。
- creates を書かない run は、bootstrap を実行するたびに実行される。何度実行しても問題ない内容にするのは利用者の責任とし、README にそう書く。

### 8.5 確認

準備コマンドは任意のプログラムを実行するため、実行前に計画を表示して確認する。確認は「Yes / No」の2択とし、No なら何も変更せず終了コード0で終える。

- creates によってスキップされるものしかない場合は確認しない。
- `--yes` を付けると確認を省略する。
- TTY がなく `--yes` もない場合は、実行する準備コマンドがあれば変更前にエラー終了する。
- 確認プロンプトで中断された場合は v0.1.0 と同じく終了コード130とする。

### 8.6 表示と失敗時の扱い

```
Repository: ~/dotfiles

Checks
  ✓ git
  ✓ symlink

Directories
  ✓ ~/.cache/zsh
  + ~/.local/bin

Run
  ✓ sh scripts/install-nix.sh    exists: ~/.nix-profile
  + sh scripts/setup-fonts.sh

1 directory created, 1 command run, 2 already done

Next: dots apply --dry-run
```

- 準備コマンドが出力した内容は、その項目の行の後にそのまま流れる。dots は加工しない。
- 途中で失敗した場合は、失敗した項目を `! failed` とし、未処理の項目を `! not processed` として表示する。そのあとに、失敗の詳細（コマンドの終了コード、作れなかったディレクトリと理由）と、「原因を取り除いて `dots bootstrap` を再実行すれば、完了済みのディレクトリと creates 付きのコマンドはスキップされる」旨を表示する。
- 前提条件の確認で失敗した場合は、Checks の該当項目を `! missing` などで示し、それ以降はすべて `! not processed` とする。
- 完了済みの手順は元に戻さない。
- `--dry-run` では Directories と Run を `+`（予定）と `✓`（不要）で示し、何も実行しない。

## 9. dots add

### 9.1 使い方

```
dots add <path> <dest>
dots add <path> <dest> --os <os>
dots add <path> <dest> --auto-mode <dir|ignore>
dots add <path> <dest> --dry-run
```

`<path>` にある既存のファイルまたはディレクトリを、リポジトリ内の `<dest>` へ移し、`<path>` に `<dest>` を指すリンクを作る。必要なら dots.toml にルールを追加する。

```sh
dots add ~/.config/nvim/init.lua ~/dotfiles/config/nvim/init.lua
dots add ~/.config/nvim/init.lua config/nvim/init.lua
```

リポジトリが ~/dotfiles なら、この2つは同じ意味になる。

| 引数 | 意味 | 設定上の名前 | 解決の基準 |
| --- | --- | --- | --- |
| `<path>` | 管理に加えたい既存の項目（移動元） | target | 絶対パス、`~` で始まるパス、またはカレントディレクトリからの相対パス |
| `<dest>` | リポジトリ内の移動先 | source | 絶対パスか `~` で始まるパスならそのまま。それ以外の相対パスはリポジトリルートを基準にする（カレントディレクトリではない） |

dots.toml の用語では、`<path>` が配置先（target）、`<dest>` が source になる。`mv` と同じ「移動元、移動先」の順に並べるため、引数名は source と target を使わない。

- `--os <os>`: ルールを `[dots]` ではなく `[dots.<os>]` に追加する。OS名は v0.1.0 の5.1の規則に従う。
- `--auto-mode <dir|ignore>`: 9.3 の確認で選ぶ方法を先に指定する。TTY がない環境で使う。

`<path>` に書けるのはホームディレクトリ配下だけである。設定ファイルの配置先は `~` で始まるか、リポジトリからの相対パスで書く（v0.1.0 の4章）。一方、リポジトリ配下の `<path>` は9.4で禁止しているためである。

### 9.2 auto との関係

`<dest>` が auto ルールの source の中に入ると、apply のときに auto がその項目をリンクする。そのため、`<dest>` の位置によって dots.toml に追加する内容が変わる。

以下、現在のOSで有効な auto ルールの source を S、target を T とする。また `<dest>` を含む S 直下の項目名を N とする。例はすべて次の設定で考える。

```toml
[[auto]]
source = "config"
target = "~/.config"
```

| 場合 | 例 | 動作 |
| --- | --- | --- |
| 1. `<dest>` がどの auto の S にも入らない | `dots add ~/.vimrc vim/vimrc` | `[dots]` に `"~/.vimrc" = "vim/vimrc"` を追加する |
| 2. `<dest>` が S 直下（S/N）で、`<path>` が T/N と一致する | `dots add ~/.config/wezterm config/wezterm` | dots.toml は変えない。auto が ~/.config/wezterm をリンクする |
| 3. `<dest>` が S 直下（S/N）だが、`<path>` が T/N と一致しない | `dots add ~/.vimrc config/vimrc` | 利用者に確認する（9.3）。そのままだと auto が ~/.config/vimrc も作ってしまう |
| 4. `<dest>` が S/N の中（2階層以上深い）で、S/N がまだリポジトリにない | `dots add ~/.config/nvim/init.lua config/nvim/init.lua` | 利用者に確認する（9.3） |
| 5. `<dest>` が S/N の中で、S/N がすでにあり、auto が T/N をリンクしている | `dots add ~/.vimrc config/nvim/vimrc`（config/nvim がある） | エラー。~/.config/nvim/vimrc としても見えてしまうため、別の移動先を指定するよう案内する |
| 6. `<dest>` が S/N の中で、S/N が ignore または os_only で除外されている | `ignore = ["nvim"]` のときの `dots add ~/.config/nvim/init.lua config/nvim/init.lua` | 1と同じく `[dots]` に追加する。auto は S/N を扱わない |

4の例を詳しく書く。~/.config/nvim は dots 管理外の普通のディレクトリで、init.lua と lua/ がある。リポジトリの config/ には fish/ と wezterm/ しかない。ここで init.lua だけを config/nvim/init.lua へ移し、`[dots]` に1行追加するとどうなるか。リポジトリに config/nvim/ ができるので、auto は次の apply で ~/.config/nvim 全体を config/nvim へのリンクにしようとする。これは `[dots]` の ~/.config/nvim/init.lua と親子関係で衝突し、設定エラーになる（v0.1.0 の5.4）。仮に衝突しなくても、~/.config/nvim の lua/ などはバックアップされてホームから見えなくなる。

- 判定には、上書きされたものを含め、現在のOSで有効なすべての auto ルールを使う。`<dest>` が複数のルールの S に入る場合は、それぞれについて判定する。
- `<dest>` が他のOSの auto ルール（例: linux で実行したときの `[[auto.darwin]]`）の S に入る場合はエラーにする。そのOSで apply したときの結果を、このOSでは確認できないためである。
- 比較は v0.1.0 の5.4と同じ正規化で行う（Windows と macOS では大文字・小文字を区別しない）。

### 9.3 auto の中に移すときの確認

9.2 の3と4の場合は、次のどれにするかを確認する。

```
config/nvim/init.lua is inside config/nvim, which [[auto]] auto[0] links to ~/.config/nvim.

> Move the whole ~/.config/nvim into config/nvim (auto links it)
  Move only init.lua, add "nvim" to ignore of auto[0], and add a [dots] rule
  Cancel
```

| 選択肢 | 動作 | 選べる条件 |
| --- | --- | --- |
| ディレクトリごと移す（`dir`） | `<path>` ではなく T/N（~/.config/nvim）全体を S/N（config/nvim）へ移し、T/N にリンクを作る。dots.toml は変えない。auto が T/N をリンクする | 4の場合で、`<path>` が T/N の中にあり、T/N からの相対位置と S/N からの `<dest>` の相対位置が同じとき（例: ~/.config/nvim/init.lua と config/nvim/init.lua）。T/N は実ディレクトリでなければならない |
| 項目だけ移す（`ignore`） | `<path>` を `<dest>` へ移す。その auto ルールの ignore に N を追加し、`[dots]`（`--os` があれば `[dots.<os>]`）に `"<path>" = "<dest>"` を追加する | 3と4の両方 |
| やめる | 何も変更せず終了コード0で終える | 常に |

- 「ディレクトリごと移す」では、`<path>` 以外のファイル（例: ~/.config/nvim/lua/）も一緒にリポジトリへ移る。確認の画面と --dry-run で、移るディレクトリを示す。
- 「項目だけ移す」では、N はその auto ルールから除外される。後で S/N に別のファイルを置いても、auto はリンクしない。共通の `[[auto]]` の ignore に追加すると、他のOSでも除外される。
- 選べる選択肢が「やめる」のほかに1つしかない場合も、確認は省略しない。
- `--auto-mode dir` または `--auto-mode ignore` を指定すると確認しない。指定した方法を選べない場合はエラーにする。
- TTY がなく `--auto-mode` もない場合は、変更前にエラー終了する。
- `--dry-run` では確認せず、選べる方法とそれぞれの結果を表示する。
- 中断は終了コード130とする。

### 9.4 変更前に止める条件

次のいずれかに当たる場合は、何も変更せず理由を表示してエラー終了する。パスの比較は v0.1.0 の5.4と同じ正規化で行う。

- `<path>` が存在しない。
- `<path>` がシンボリックリンクである（dots が作ったリンクなら「すでに管理されている」と表示する）。
- `<path>` の親のいずれかが、dots が管理しているリンクである（例: ~/.config/nvim がリンクのとき ~/.config/nvim/init.lua）。中身はすでにリポジトリにある。
- `<path>` がリポジトリルートの中にある、またはリポジトリルートを含む。ホームディレクトリの外にある。
- `<path>` が通常ファイルでもディレクトリでもない。ディレクトリの中に通常ファイル、ディレクトリ、シンボリックリンク以外の項目がある場合も同じ。
- `<dest>` がリポジトリの外にある。リポジトリルートそのもの、dots.toml、.git とその中を指している。
- `<dest>`（「ディレクトリごと移す」では S/N）に、すでに何かが存在する。`<dest>` の親に通常ファイルがある。
- `<path>` が、現在の設定のいずれかのルール（上書きされたものを含む）の配置先と同じである。または親子関係で衝突する（v0.1.0 の5.4）。
- 9.2 の5の場合、または他のOSの auto ルールの S に入る場合。
- source と target から相対リンクを作れない（Windows で別ボリュームにある場合など）。
- Windows でシンボリックリンクを作る権限がない。
- dots.toml の書き換えが6.1の方法でできない。
- 移動と dots.toml の書き換えを行った後の状態で設定を展開し直したとき、配置先の集合が「元の配置先＋今回の配置先1件」にならない。今回の配置先は、「ディレクトリごと移す」では T/N、それ以外では `<path>` である。展開し直しは、移動先に項目があるものとして計画の段階で行う。9.2 の判定に漏れがあった場合の最後の安全確認である。

### 9.5 処理の順序と巻き戻し

1. 9.4 の確認を行い、必要なら9.3の確認を行う。
2. 移す項目（`<path>`、「ディレクトリごと移す」では T/N）を移動先へ移す。移動先の親ディレクトリは必要に応じて作る。同じボリュームなら rename する。rename できない場合（別のファイルシステム）は、移動先へコピーしてから元を削除する。コピーが終わる前に失敗した場合は、コピー途中のものを削除する。
3. dots.toml を書き換える（ルールや ignore を追加する場合だけ）。
4. 元の場所に相対シンボリックリンクを作る。
5. state にリンクを記録する。

2〜4のどこかで失敗した場合は、済んだ手順を逆の順に戻す。リンクを消し、dots.toml を元のバイト列に戻し、ファイルを元の場所へ戻す。2で作った親ディレクトリは、空なら削除する。戻すのにも失敗した場合は、各ファイルが今どこにあるか（元の内容がある場所、書き換え前の dots.toml の内容を保存した一時ファイル）を表示する。5の失敗は警告にとどめ、2〜4は戻さない。次回の apply で記録される。

別ボリュームの場合も、元を削除するのは移動先へのコピーが完了した後だけである。巻き戻しに失敗しても内容は移動先に残っており、どの時点でもデータが失われることはない。

add はバックアップアーカイブを作らない。元のファイルは削除せず移動するだけで、失敗時は上の手順で戻すためである。

### 9.6 表示

```
Repository: ~/dotfiles

  + config/nvim/init.lua     moved from ~/.config/nvim/init.lua
  + dots.toml                add "nvim" to ignore of auto[0] (all OSes)
  + dots.toml                [dots] "~/.config/nvim/init.lua" = "config/nvim/init.lua"
  + ~/.config/nvim/init.lua  ← config/nvim/init.lua

Added ~/.config/nvim/init.lua
```

- 移動、dots.toml の書き換え、リンクの作成を、行う順に1行ずつ示す。dots.toml を変えない場合は、その行を出さない。
- 共通ルールの ignore を書き換える場合は `(all OSes)` を付ける。
- `--dry-run` でも同じ行を表示し、最後の行を `Nothing was changed (dry run).` にする。
- 失敗した場合は、失敗した手順を `! failed` とし、巻き戻した手順を `~ reverted` として表示する。

## 10. dots unlink

### 10.1 使い方

```
dots unlink <target>...
dots unlink --stale
dots unlink --stale --yes
dots unlink ... --dry-run
```

- `<target>` は配置先のパス。add の `<path>` と同じ形式を受け付ける。複数指定できる。
- `--stale` は、stale（5.4）を対象にする。
- `<target>` と `--stale` は同時に指定できない。何も指定しない場合も使い方の誤り（終了コード2）とする。

2つの使い方は、dots.toml を変えるかどうかが違う。

| 使い方 | リンク | dots.toml |
| --- | --- | --- |
| `<target>` | 外す | 指定した配置先を管理から外すように書き換える（10.3） |
| `--stale` | 外す | 変えない。ルールはすでに消えている |

`<target>` は「この配置先の管理をやめる」ための使い方なので、次の apply で作り直されないように dots.toml も書き換える。

現在の設定のリンクをすべて外す `--all` は設けない。一度の操作で影響する範囲が広すぎるためである。複数の配置先をまとめて外す場合は `<target>` を複数指定する。

### 10.2 `<target>` の対象の決め方

dots は、`[dots]` のエントリーも `[[auto]]` の項目も、配置先を1つのリンクとして扱う。ディレクトリの中の個々のファイルは管理していない。そのため、`<target>` と現在の設定の配置先を次のように照らし合わせる。比較は v0.1.0 の5.4と同じ正規化で行う。

1. 現在の設定で採用されている配置先と一致する: その配置先を対象にする。
2. 採用されている配置先の下にある（例: ~/.config/nvim が配置先のときの ~/.config/nvim/init.lua）: 下記の確認を行う。
3. stale の target と一致する: stale として扱う（10.4の表）。dots.toml は変えない。
4. どれにも当たらない: 変更前にエラー終了する。配置先の親ディレクトリ（例: auto の target である ~/.config）を指定した場合もここに当たる。

2の場合は、指定したパスだけを管理から外すことはできないので、上位の配置先をどうするか確認する。

```
~/.config/nvim/init.lua is inside ~/.config/nvim, which dots links as a whole.

> Unlink ~/.config/nvim
  Skip ~/.config/nvim/init.lua
```

- 「Unlink」を選ぶと、上位の配置先（~/.config/nvim）を1の場合と同じく対象にする。
- 「Skip」を選ぶと、そのパスについては何もしない（`! skipped`）。
- 複数の `<target>` が同じ上位の配置先の下にある場合は、1回だけ確認する。
- TTY がない場合は確認できないので、変更前にエラー終了し、上位の配置先を直接指定するよう案内する。`--yes` はこの確認には効かない。
- `--dry-run` では確認せず、`! inside ~/.config/nvim` として上位の配置先を示す。
- 中断は終了コード130とする。

### 10.3 dots.toml の書き換え

`<target>` の対象になった配置先ごとに、現在のOSでその配置先を出力するルールをすべて書き換える。上書きされて採用されていない候補も含める。採用されているルールだけを消すと、上書きされていた下位のルールが次の apply で有効になってしまうためである。

| 出力しているルール | 書き換え方 |
| --- | --- |
| `[dots]`、`[dots.<現在のOS>]` のエントリー | そのエントリーの行を削除する |
| `[[auto]]`、`[[auto.<現在のOS>]]` のルール | そのルールの ignore に、source 直下の項目名を追加する。ignore がなければ `ignore = ["<名前>"]` の行を追加する |

例: 次の設定で `dots unlink ~/.vimrc ~/.config/nvim` を実行する。

```toml
[dots]
"~/.vimrc" = "vimrc"

[[auto]]
source = "config"
target = "~/.config"
ignore = ["README.md"]
```

書き換え後は次のようになる。

```toml
[dots]

[[auto]]
source = "config"
target = "~/.config"
ignore = ["README.md", "nvim"]
```

- `[dots]` と `[[auto]]` の共通ルールを書き換えると、他のOSでもその配置先は配置されなくなる。結果表示と --dry-run で、共通ルールを書き換えることを示す。
- 他のOSのルール（例: linux で実行したときの `[dots.darwin]`）は書き換えない。
- 項目名に `*`、`?`、`[`、`{` が含まれる場合、ignore のパターンとして正確に書けない（v0.1.0 の5.3）。この場合は変更前にエラー終了し、dots.toml を手で編集するよう案内する。
- 書き換えは6.1の規則に従う。さらに、書き換え後の設定を展開し直し、配置先の集合が「元の配置先から今回の対象を除いたもの」と一致することを確かめてから書き込む。
- 配置先の状態が NotExist、InvalidLink、FileOrDir でも、dots.toml は書き換える。リンクがなくても、管理をやめるという指定は有効だからである。

### 10.4 リンクの外し方

対象ごとに、v0.1.0 の6章の状態（stale は5.4の状態）で判定する。

| 対象 | 状態 | リンク |
| --- | --- | --- |
| 設定にある配置先 | ValidLink | 削除し、state の記録を消す |
| 設定にある配置先 | NotExist | 何もしない（`✓ not linked`） |
| 設定にある配置先 | InvalidLink、FileOrDir | 触らない（`! skipped`） |
| stale | StaleLink | 削除し、state の記録を消す |
| stale | StaleChanged | 触らない（`! skipped`）。state の記録を消す |
| stale | StaleGone | state の記録を消す（表示はしない） |

- 削除するのはシンボリックリンクそのものだけである。source 側のファイル、リンク先、通常ファイル、ディレクトリは削除しない。
- 削除の直前にもう一度 Lstat と Readlink で状態を確かめ、判定時と違っていれば削除せず `! skipped` とする。
- apply が作った親ディレクトリは、空になっても削除しない。
- バックアップからの復元は行わない。案内だけを表示する（10.6）。

`! skipped` は、dots が作ったリンクがそこにない、という望む状態にすでになっているので、エラーとは扱わない。終了コードは、I/O エラーや対象外の `<target>` がなければ0とする。

### 10.5 確認と処理の順序

- `--stale` では、外す対象と触らない対象を一覧したうえで「Yes / No」を確認する。一括で外すと、stale の判定に誤りがあったときに影響が広いためである。No なら何も変更せず終了コード0で終える。`--yes` で確認を省略できる。TTY がなく `--yes` もない場合は、外す対象があれば変更前にエラー終了する。中断は終了コード130とする。
- `<target>` では、10.2 の上位の配置先の確認を除いて確認しない。利用者が対象を名指ししており、dots.toml の変更は git で戻せ、source も消えないためである。事前の確認には --dry-run を使う。

`<target>` の処理は次の順に行う。

1. すべての `<target>` について対象を決め（10.2）、dots.toml の書き換え内容を計算して検証する（10.3）。ここで問題があれば何も変更しない。
2. dots.toml を書き換える。複数の `<target>` の変更は1回の書き込みにまとめる。
3. リンクを外す（10.4）。
4. state を更新する。

dots.toml を先に書き換えるのは、リンクを外す途中で失敗したときに、残ったリンクが stale として検出され、`dots unlink --stale` で片付けられるようにするためである。3で失敗した場合は、その旨と `dots unlink --stale` を案内する。

### 10.6 表示

```
Repository: ~/dotfiles

  - ~/.vimrc               ← vimrc
    dots.toml: remove "~/.vimrc" = "vimrc" from [dots] (all OSes)
  - ~/.config/nvim         ← config/nvim
    dots.toml: add "nvim" to ignore of auto[0] (source "config", all OSes)
  ✓ ~/.config/wezterm     not linked
    dots.toml: remove "~/.config/wezterm" = "config/wezterm" from [dots.darwin]
  ! skipped ~/.gitconfig
    a regular file exists; dots did not create it
    dots.toml: remove "~/.gitconfig" = "git/gitconfig" from [dots] (all OSes)

2 unlinked, 1 not linked, 1 skipped, dots.toml updated

Backup available for ~/.vimrc:
  dots restore ~/.local/share/dots/backups/20261001T120000Z-0001-vimrc.tar.gz
```

- 削除は新しい記号 `-` で示す（13章）。
- dots.toml の書き換えは、各項目の詳細行に表示する。共通ルールを書き換える場合は `(all OSes)` を付ける。
- stale の項目には `stale` を付ける。
- 外した配置先について、バックアップディレクトリにその配置先のアーカイブがあれば、最も新しいものを `dots restore` の形で案内する。アーカイブの manifest.json だけを読み、復元は行わない。アーカイブが読めない場合はその旨を出さずに飛ばす。

### 10.7 v0.2.0 で決めたこと

草案の Issue 4 では「設定からルールを消すか、バックアップを戻すかも、このコマンドに含めるか決める」としていた。v0.2.0 では次のとおりとする。

- `<target>` を指定した場合は、ルールの削除（dots）または ignore への追加（auto）まで行う（10.3）。`--stale` では dots.toml を変えない。現在の設定のリンクをすべて外す機能は設けない。
- バックアップの復元は含めない。案内だけを表示する（10.6）。

## 11. dots cd

```
dots cd
```

- 4章の方法で決めたリポジトリルートの絶対パスを、OSのパス形式で標準出力に1行だけ表示する。Windows では `C:\Users\user\dotfiles` のような形式になる。
- ホームディレクトリを `~` に省略しない。色や装飾も付けない。標準出力が TTY でも同じとする。
- dots.toml が見つからない場合は、標準出力には何も出さず、4章のエラーを標準エラーに表示して終了コード1で終える。
- シェル自体のディレクトリは変えられないので、README に次の使い方を書く。dots cd が失敗して何も出力しなかったとき、fish の `cd` は引数なしとなってホームディレクトリへ移動してしまう。そのため、どのシェルでも成功したときだけ移動する書き方を案内する。

```sh
d="$(dots cd)" && cd "$d"                          # bash, zsh
set d (dots cd); and cd $d                         # fish
$d = dots cd; if ($?) { Set-Location $d }          # PowerShell
```

## 12. 既存コマンドの変更

### 12.1 doctor

doctor は、v0.1.0 の診断に加えて stale を表示する。

```
Repository: ~/dotfiles

  ! stale ~/.config/tmux  ← config/tmux
    not in dots.toml; created by dots on 2026-10-01
  ! stale ~/.zshrc
    changed since dots created it; dots will not remove it

Run "dots unlink --stale" to remove stale links.
```

- StaleLink は `! stale` とし、記録の source を `←` の後に表示する（12.3）。
- StaleChanged は `! stale` とし、詳細に変更されていることを表示する。unlink --stale では削除されないことも示す。
- StaleGone は表示しない。doctor は state を書き換えないので、記録はそのまま残る。
- StaleLink と StaleChanged があれば、doctor の終了コードは1とする。`!` は要対応を表すという v0.1.0 の8章の約束に合わせる。state が壊れている場合（5.5）も1とする。

### 12.2 apply

- apply は 5.3 のとおり state を更新する。
- stale は削除しない。stale が1件以上あれば、要約の後に件数と `Run "dots unlink --stale" to remove them.` を表示する。
- state の書き込みに失敗しても、作ったリンクは戻さない。警告を表示し、終了コードは1とする。

### 12.3 変更した項目の source を表示する（#9）

#### 現象

apply の結果表示には配置先だけが出る。どの source から来たリンクかは、doctor を実行しないと分からない。

```text
Repository: ~/dotfiles

  ✓ ~/.config/fish
  ✓ ~/.config/nvim
  ✓ ~/.config/starship.toml
  + ~/.config/wezterm

1 created, 3 already ok
```

#### 修正案

変更した項目の行にだけ、source を `←` の後に表示する。普段の出力は簡潔なまま、変更があったときだけ出所が分かるようにする。

```text
Repository: ~/dotfiles

  ✓ ~/.config/fish
  ✓ ~/.config/nvim
  ✓ ~/.config/starship.toml
  + ~/.config/wezterm       ← config/wezterm

1 created, 3 already ok
```

- 対象は「変更した」項目とする。apply と apply --dry-run の `+`（作成）と `~`（置換）、add のリンク作成の行、unlink の `-`（削除）の行に付ける。`✓` と `!` の行には付けない。
- source はリポジトリルートからの相対パスで、/ 区切りで表示する（v0.1.0 の8章の表示規則と同じ）。stale の項目は state に記録した source を使う。
- `linked to` のような語は使わず、`←` だけを使う。
- source は薄い色（dim）で表示する。色なしの場合（`NO_COLOR`、TTY でない場合）も `← source` の文字は出す。
- `←` の位置は、その一覧で `←` を付ける行のうち最も長い「記号＋配置先」に合わせて揃える。幅は表示幅（全角文字は2）で数える。揃えた結果が端末幅を超える場合は、揃えずに配置先の後ろに空白2つで続ける。
- doctor の source の表示（v0.1.0 の8章の `-> source`）も `←` に変える。矢印の向きと意味を、すべてのコマンドで「配置先 ← source」にそろえるためである。doctor は診断用なので、これまでどおり全項目に source と出所を表示する。変更した項目だけに付ける規則は、apply、add、unlink に適用する。

これに合わせて、add の表示（9.6）のリンク作成の行も `+ ~/.config/nvim/init.lua  ← config/nvim/init.lua` とし、移動の行は `+ config/nvim/init.lua  moved from ~/.config/nvim/init.lua` とする。`←` の意味を「配置先 ← source」の1つにそろえるためである。

#### やること

- [ ] ui の項目行に、変更した項目だけ source を付ける処理を追加する（apply、apply --dry-run）。
- [ ] `←` の列揃えと、端末幅を超える場合の扱いを実装する。
- [ ] add と unlink の表示を同じ規則で実装する。
- [ ] doctor の `-> source` を `← source` に変える。
- [ ] README の出力例を更新する。
- [ ] 単体テスト: 変更した項目だけに付くこと、列揃え、全角文字を含むパス、色なしの出力。

#### 完了の条件

- apply の結果で、`+` と `~` の行にだけ `← <source>` が出る。
- 色なしの出力に ANSI エスケープが含まれない。

### 12.4 競合の確認プロンプトの修正（#11）

#### 現象

apply の競合の確認で、「Process the rest of the files similarly」（以降の競合にも同じ答えを使う）のチェックボックスに、上下キーでカーソルを移せない。

```text
┃ ~/.claude/settings.json already exists as existing file or directory.
┃ Replace it with a link to ~/dotfiles/config/agents/claude/settings.json?
┃   Yes – back up and replace
┃ > No – skip

  > • Process the rest of the files similarly

↑ up • ↓ down • / filter • enter select
```

意図していたのは、最初はオフのチェックボックスで、上下キーで移動でき、オンとオフを切り替えられる形である（v0.1.0 の6章「競合時の対話」の Yes to All／No to All）。

#### 原因

internal/ui/prompt.go の `Confirm` は、huh のフォームに Select（Yes／No）と MultiSelect（チェックボックス1つ）の2つの入力欄を並べている。huh では、上下キーは今いる入力欄の中だけで動く。入力欄の間を移るには Tab か Enter を使う。Select で Enter を押すと選択が確定して次の入力欄へ移るが、画面のヘルプには上下キーと Enter しか出ないので、チェックボックスへ移れないように見える。

加えて、huh の既定テーマでは、チェックの入っていない項目が `•` で描かれる。そのためチェックボックスに見えない。2つの入力欄にそれぞれ `>` のカーソルが出ていることも、どちらを操作しているのかを分かりにくくしている。

#### 修正案

huh をやめ、bubbletea の上に確認プロンプトの部品を1つ書く。端末の raw モード、OSごとのキー入力、描画、Ctrl-C の受け取りは bubbletea に任せ、dots はキー入力を受けて状態を変える処理（Update）と表示を作る処理（View）だけを持つ。OSごとの処理は書かない。

1つの画面の中で、上下キーで選択肢とチェックボックスの間を移動できるようにする（画面は下記）。

- 上下キー（と j／k）で、選択肢とチェックボックスの間を移動する。チェックボックスは最初はオフである。
- チェックボックスの上で Space を押すと、オンとオフが切り替わる（`[ ]` と `[x]`）。
- Enter で確定する。カーソルが選択肢の上ならその選択肢を、チェックボックスの上なら直前に選んでいた選択肢（初期値は No）を答えとする。答えは、選んだ選択肢とチェックボックスの状態の組み合わせで決まる（v0.1.0 の6章の4通り）。
- 答えが確定したかどうかを部品自身が持つ。Enter で確定していない終わり方（Ctrl-C、SIGTERM、SIGHUP、入力の終わり）はすべて中断とし、終了コード130にする。huh では回答なしの終了を区別できず、既定値が使われた（#2）。部品が確定の有無を持つことで、これが起きないようにする。SIGHUP の扱い（#3）は今の `runForm` と同じ方法を引き継ぐ。

部品の範囲は次に固定する。保守する量を増やさないためである。

- 作るもの: 質問文、選択肢の一覧（2〜4個）、任意のチェックボックス1つ、操作のヘルプ1行。
- 作らないもの: 絞り込み検索、スクロール、複数のチェックボックス、自由入力、マウス操作、テーマの切り替え。
- 確認はすべてこの部品で作る。apply の競合（`Confirm`）、restore（`ConfirmRestore`）、v0.2.0 で増える bootstrap の Yes／No（8.5）、add の auto の確認（9.3）、unlink の上位の配置先の確認（10.2）と `--stale` の Yes／No（10.5）である。確認ごとに個別の画面は書かない。

huh を使わなくなるので、huh は依存から外す。bubbletea は残る。

#### 画面

すべての確認を同じ形で表示する。質問文、選択肢、（あれば）チェックボックス、操作のヘルプの順に並べる。

```text
~/.vimrc already exists as a regular file.
Replace it with a link to vimrc?

  > Yes – back up and replace
    No – skip
    [ ] Process the rest of the files similarly

  ↑/↓ move · space toggle · enter confirm · ctrl+c cancel
```

- カーソルのある行は `>` と強調色で示す。チェックボックスは `[ ]`（オフ）と `[x]`（オン）で示す。ヘルプは薄い色で、チェックボックスがない確認では `space toggle` を出さない。
- 色なし（`NO_COLOR`）でも `>`、`[ ]`、`[x]` は出すので、操作できる。
- 初期カーソルは、何も変更しない選択肢（No、Skip、Cancel）に置く。Enter を押し続けても何も変更されないようにするためである。チェックボックスは常にオフで始める。
- 確定すると、確認の表示を消して、答えを1行だけ残す。後から端末を見返したときに、何を選んだか分かるようにするためである。

```text
? ~/.vimrc  Yes – back up and replace (and the rest)
```

- 中断した場合は `? ~/.vimrc  cancelled` を残す。
- 質問文は1回だけ書き、書き直すのは選択肢とチェックボックスの行だけにする。長いパスで質問文が折り返しても、表示が崩れないようにするためである。

v0.2.0 の確認は次の6つになる。

| 確認 | 選択肢 | チェックボックス | 初期カーソル |
| --- | --- | --- | --- |
| apply の競合（v0.1.0 の6章） | Yes – back up and replace / No – skip | Process the rest of the files similarly | No |
| restore の既存ターゲット（v0.1.0 の7.2） | Back up current content and restore / Skip | なし | Skip |
| bootstrap の準備コマンド（8.5） | Yes – run them / No – stop here | なし | No |
| add の auto の中への移動（9.3） | Move the whole … / Move only … / Cancel | なし | Cancel |
| unlink の上位の配置先（10.2） | Unlink … / Skip … | なし | Skip |
| unlink --stale（10.5） | Yes – remove them / No – keep them | なし | No |

bootstrap の例:

```text
Run 2 setup commands in ~/dotfiles?
  sh scripts/install-nix.sh
  sh scripts/setup-fonts.sh

  > Yes – run them
    No – stop here

  ↑/↓ move · enter confirm · ctrl+c cancel
```

add の例:

```text
config/nvim/init.lua is inside config/nvim, which auto[0] links to ~/.config/nvim.
How do you want to add ~/.config/nvim/init.lua?

    Move the whole ~/.config/nvim into config/nvim (auto links it)
    Move only init.lua, ignore "nvim" in auto[0], and add a [dots] rule
  > Cancel

  ↑/↓ move · enter confirm · ctrl+c cancel
```

選べない選択肢（9.3 の条件を満たさない「ディレクトリごと移す」など）は表示しない。

#### #4 との関係

#4（先行入力で確認プロンプトが止まる）の原因は、bubbletea v1 がパッケージの初期化時に端末へ背景色を問い合わせることである。v2 ではこの問い合わせが初期化時に行われないので、部品を bubbletea v2 で書けば #4 も同時に解決する。

bubbletea は v2 を使う。v1 で書いて後から v2 へ移る手間を省くためでもある。

- インポートパスは `charm.land/bubbletea/v2` である（v2.0.0 は 2026年2月に安定版としてリリースされた）。v2 のソースには v1 の `HasDarkBackground` の呼び出しがないことを確認した。
- v2 では、キー入力は `tea.KeyPressMsg`、View の戻り値は `tea.View` になる。`WithInput`、`WithOutput`、`WithContext`、`tea.ErrInterrupted` は v2 でも使える（調査の要約による。実装時に確かめる）。
- v2 は版によって必要な Go が上がっている。v2.0.0〜v2.0.2 は Go 1.24.2、v2.0.3〜v2.0.9 は Go 1.25、v2.0.10 は Go 1.26 である。古い版に固定すると以降の修正を取り込めないので、dots の最小 Go を 1.26 に上げ、bubbletea は最新の v2 を使う。Go のサポートは最新の2つのメジャーバージョン（2026年10月時点で 1.26 と 1.27）なので、1.24 と 1.25 はすでにサポート外である。

#### やること

- [ ] go.mod の `go` を 1.26.0 に上げ、bubbletea を `charm.land/bubbletea/v2` の最新版にする。
- [ ] CI で Go 1.24 を指定しているジョブ（`.github/workflows/ci.yml`）を 1.26 にする。README、PKGBUILD、flake.nix に書かれた最小 Go の記述も合わせる。
- [ ] 確認プロンプトの部品を書く（Update と View）。選択肢の一覧と、任意のチェックボックスを持てるようにする。画面は上記のとおりとする。
- [ ] 部品の単体テストを書く。キー入力のメッセージを Update に渡し、カーソル、チェック、確定の状態を確かめる。端末は使わない。
- [ ] 確定していない終わり方がすべて中断（終了コード130）になることをテストする。
- [ ] apply の競合の確認（`Confirm`）と restore の確認（`ConfirmRestore`）を置き換える。
- [ ] huh を go.mod から外す。
- [ ] v0.2.0 の新しい確認（8.5、9.3、10.2、10.5）をこの部品で作る。
- [ ] tty-e2e の expect シナリオを更新する。下キーでチェックボックスへ移れること、Space で切り替えて Yes to All になること、Ctrl-C で130になることを確認する。
- [ ] #4 の再現手順（`printf '\r' | script -qec "dots apply" /dev/null`）でプロンプトが止まらないことを確認する。

#### 完了の条件

- 競合の確認で、上下キーだけでチェックボックスへ移動でき、Space で切り替えられる。
- チェックボックスをオンにして Yes を選ぶと、以降の競合が確認なしでバックアップ付き置換される。No も同様にスキップされる。
- Enter で確定せずに終わった確認は、すべて終了コード130になる。
- #4 の再現手順でプロンプトが止まらない。
- Linux と macOS の tty-e2e が通る。Windows の CI で、ビルドと部品の単体テストが通る。

## 13. 表示と記号

v0.1.0 の8章の記号に次を加える。

| 記号 | 色 | 意味 | 使うコマンド |
| --- | --- | --- | --- |
| `-` | 赤 | 削除（リンクを外す） | unlink |
| `+` | 青 | 作成（ディレクトリ、移動、リンク、ルール） | bootstrap、add（既存の意味を広げる） |
| `✓` | 緑 | すでに望む状態にある | bootstrap、unlink（既存の意味を広げる） |
| `!` | 黄 | 問題あり。新しい短い語として `stale`（doctor）、`missing`（bootstrap）、`inside`（unlink --dry-run）を加える。`failed`、`not processed` は bootstrap と add でも、`skipped` は unlink でも使う | doctor、bootstrap、add、unlink |
| `~` | 紫 | 巻き戻した（`~ reverted`） | add |

- 項目行の形（2スペースの字下げ、記号、パス、詳細は4スペースの字下げ）は v0.1.0 と同じとする。
- 色なしの扱い、エラーメッセージ中のパスの省略も v0.1.0 と同じとする。
- dots cd だけは11章のとおり、装飾のないパスを出す。

## 14. テスト方針

v0.1.0 の9章の方針を続ける。memfs で次を単体テストする。

- 4章のリポジトリ解決（`--repo`、`DOTS_REPO`、カレントディレクトリ、記録の優先順位、指定先に dots.toml がない場合、記録が壊れている場合）と、記録を書く条件（init は記録しない、など）。
- state の読み書き、壊れたファイルの退避、新しい version の拒否、記録と削除の規則。
- stale の判定（StaleLink、StaleChanged、StaleGone、別リポジトリの記録の除外）。
- bootstrap の設定検証、手順の順序、directories の状態ごとの動作、creates によるスキップ、失敗時の not processed。
- add の引数の解決（`<dest>` の絶対パスとリポジトリ基準の相対パス）、9.2 の6つの場合、9.3 の選択肢と選べる条件、`--auto-mode`、止める条件、dots.toml のテキスト編集（コメント保持、見出しの有無、ignore の追加と新設、CRLF、インラインテーブルの拒否）、各手順での失敗を注入した巻き戻し。
- unlink の対象選択（一致、配置先の下のパス、stale、どれにも当たらない）、dots.toml の書き換え（エントリー削除、ignore の追加と新設、上書きされた候補も含めること、ignore に書けない名前）、状態ごとの動作、削除直前の再確認。
- bootstrap <url> の取得先の決め方、取得済みの場合のスキップ、既存の取得先での停止、URL のユーザー情報の伏せ字。git の実行は差し替え可能にしてテストする。

CI の実バイナリE2E（smoke.sh）に次のシナリオを加え、5つのOSで実行する。

1. ローカルのベアリポジトリを file URL で `dots bootstrap <url>` し、`dots cd` が取得先を返すこと。同じコマンドの再実行で取得がスキップされること。
2. `dots bootstrap --dry-run` と `dots bootstrap`（directories と creates 付きの run）、再実行で何も変わらないこと。
3. `dots add` でファイルとディレクトリを取り込み、`dots doctor` が正常を返すこと。auto の中への add を `--auto-mode dir` と `--auto-mode ignore` の両方で行い、再度 apply しても何も変わらないこと。
4. dots.toml からルールを消し、`dots doctor` が stale を表示し、`dots unlink --stale` で消え、`dots doctor` が stale を表示しなくなること。
5. `dots unlink <target>` で dots のエントリーが消え、auto の ignore に項目名が入り、再度 apply してもリンクが作られないこと。
6. リポジトリ外のディレクトリで `dots doctor` と `dots cd` が記録したリポジトリを使い、`--repo` と `DOTS_REPO` がそれより優先されること。

bootstrap、add、unlink の確認プロンプト（Yes／No、add の auto の確認、unlink の上位の配置先の確認、Ctrl-C で終了コード130）は、v0.1.0 と同じく Linux と macOS で expect を使って確認する。

## 15. 未決事項と次のバージョンへの申し送り

- 複数のdotfilesリポジトリを使い分ける場合の扱い。v0.2.0 では state の記録はリポジトリごとに分けて持ち、リポジトリの記録（4.2）は1つだけである。一時的な切り替えは `--repo` と `DOTS_REPO` で行う。
- リポジトリを移動した後に、移動前の記録を新しい場所へ付け替える方法。
- unlink 時に source の中身をコピーして残すオプション（dots の利用をやめる場合向け）。
- 共通ルールを特定のOSでだけ無効にする書き方（例: `[dots.darwin]` の `"~/.vimrc" = false`、auto の OS ごとの ignore）。v0.2.0 の unlink は、共通ルールを書き換えると全OSで配置されなくなる（10.3）。このOSでだけ外したい場合は、今のスキーマでは書けない。
- `dots status`、`dots diff`（Issue #1 のコメントにあるヘルプ例）。
- #10（apply で変化のなかった項目を折りたたむ設定）。v0.2.0 に入れるかは保留。入れる場合は、設定のキー（例: `[ui] fold_unchanged`）、折りたたむ件数の閾値、#9 の `←` 表示との組み合わせを仕様に加える。
- bootstrap の準備コマンドを v0.4.0 のプラグインへ移すかどうか。v0.2.0 の `run` は設定に書いたコマンドを実行するだけにとどめ、プラグインの仕様と重ならないようにしている。
