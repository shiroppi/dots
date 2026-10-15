# dots v0.2.0 仕様設計書

## 文書情報

- 対象バージョン: v0.2.0
- 前提: v0.1.0 仕様（docs/develop/v0.1/architecture.md）。この文書に書いていないことは v0.1.0 の仕様に従う。
- 実装言語: Go 1.26 以上（v0.1.0 の 1.24 から上げる。8.4）
- 設定ファイル: dots.toml
- 元になった草案: GitHub Issue #1「v1.0までの実装草案」の「v0.2.0 — 初回セットアップと普段使い」
- 関連Issue: #6（設定から消したルールのリンクが残る）、#9（apply で変更したファイルの source を表示する）、#11（競合の確認でチェックボックスへ移れない）、#4（先行入力で確認プロンプトが止まる）、#10（apply で変化のなかった項目を折りたたむ）

## 1. 目的と範囲

v0.1.0 で、dots.toml に書いたルールどおりにリンクを作り、診断し、バックアップから戻すところまでできた。v0.2.0 では、その前後にある手作業を減らす。

- 設定から消したルールのリンクを見つけて、安全に片付ける。dots の管理をやめるときも、ルールを消してから同じ方法で外す。
- どこからでも、同じリポジトリに対して dots を実行できる。

### 1.1 草案Issueとの対応

| 草案のIssue | v0.2.0 での扱い | 本書 |
| --- | --- | --- |
| Issue 4: `dots unlink` | `dots unlink` は設けず、stale を片付ける `dots clean` だけを加える。dots.toml は書き換えない。Issue 13 と一緒に設計する | 7章 |
| Issue 12: 実機E2E（CI） | v0.1.0 で実装済み。新コマンドのシナリオを追加する | 10章 |
| Issue 13: 設定から消したルールのリンク | 含める。stateファイルを導入する | 5章、7章、8章 |

草案以外に、次の GitHub Issue も v0.2.0 で扱う。

| Issue | v0.2.0 での扱い | 本書 |
| --- | --- | --- |
| #9: apply で変更したファイルの source を表示する | 含める | 8.3 |
| #11: 競合の確認でチェックボックスへ移れない | 含める。huh をやめ、bubbletea v2 で確認プロンプトを書いて直す | 8.4 |
| #4: 先行入力で確認プロンプトが止まる | 確認プロンプトを bubbletea v2 で書くことで一緒に解決する | 8.4 |
| #10: unchanged なファイルを折りたためる設定がほしい | 含める。dots.toml に `[general]` を加え、既定で折りたたむ | 8.5 |

### 1.2 v0.2.0 の非目標

v0.1.0 の非目標（パッケージマネージャー、環境変数管理、Nixの代替、テンプレートエンジン、プロファイル切り替え）は引き続き対象外とする。加えて、次のことは v0.2.0 でやらない。

- `dots diff`。Issue #1 のコメントにあるヘルプ例には含まれているが、v0.2.0 では追加しない。
- `dots bootstrap`（初回セットアップと、Gitのurlからのリポジトリの取得）。v0.2.0 では扱わず、入れるバージョンは未定とする（#14）。
- リポジトリの更新（pull）や、一部のファイルだけの取得。前者は git を直接使う。後者は v0.4.0 の source プラグインで扱う。
- Gitへの commit、push。init も行わない。
- `dots add`（既存のファイルをリポジトリへ移して管理に加える）と `dots cd`（リポジトリへ移動する）。dots は dots.toml にリンクを宣言して適用するだけの道具で、ファイルをリポジトリへ移すことも、シェルのカレントディレクトリを変えることもしない。ファイルの移動は利用者と git に任せ、リポジトリの場所は利用者がすでに知っているので、dots の考え方に合わないとして追加しない。
- dots.toml を書き換えること。配置先の管理をやめるときは、利用者が dots.toml からルールを消し、`dots clean` でリンクを片付ける（7.5）。
- `dots edit`（dots.toml をエディターで開く）。v0.1.0 にはあったが削除する（2章）。エディターでファイルを開くのは `$EDITOR dots.toml` で足り、dots の役割ではない。`dots cd` を追加しないのと同じ考え方である。
- clean 時に、リンクを source の中身のコピーで置き換えること。
- 複数のdotfilesリポジトリを同時に使い分けること（5.4、11章）。
- プラグイン機構。
- Issue #12（親ディレクトリの扱い）。現状では `internal/apply/execute.go` の `place` が、配置先の親ディレクトリを確認なしで `MkdirAll(0o755)` で作る。v0.2.0 ではこの動作を変えない。
- Issue #13（copy モード）。v0.3.0 で扱う（docs/develop/v0.3/architecture.md）。

## 2. v0.1.0 からの変更点

| 項目 | 変更内容 | 互換性 |
| --- | --- | --- |
| リポジトリの解決 | `--repo <dir>` と `DOTS_REPO` で指定できる。dots.toml がカレントディレクトリから見つからないときは、記録したリポジトリを使う（4章） | v0.1.0 でエラーだった場面が成功するようになる。指定がなければ既存の動作は変わらない |
| リポジトリの記録 | apply がリポジトリの場所をデータディレクトリに記録する（4.2） | 新規 |
| stateファイル | apply がリンクを記録する（5章） | 新規。v0.1.0 で作ったリンクは、v0.2.0 で最初に apply したときに記録される |
| doctor → status | `dots doctor` を `dots status` に改名する。草案の `status`（管理ファイルの一覧と状態）と `doctor` は両立しないと分かったため、`status` に統合する（8.1） | `dots doctor` は使えなくなる（別名も残さない）。スクリプトは `dots status` に切り替える |
| status | stale（設定にはないが dots が作ったリンク）を表示する（8.1） | stale があると終了コード1になる。stale は v0.2.0 で記録したリンクからしか生じないので、更新直後に結果が変わることはない |
| apply | stale があると件数と片付け方を表示する（8.2）。変更した項目に `← source` を表示する（8.3） | stale を自動では消さない |
| status の source 表示と --verbose | 既定では source と出所（`auto[0]` など）を表示しない。`--verbose` を付けると全項目に `← source` と出所を表示し、`←` の位置をそろえる。v0.1.0 の `-> source` は `← source` に変わる。上書きされたルールの一覧（Overridden rules）は status では表示しない（apply --dry-run では表示する）。要約の「上書きされたルールの件数」は廃止する（8.1、8.3） | 表示の変更。v0.1.0 の doctor の出力を解析しているスクリプトには影響する |
| Go の最小バージョン | 1.24 から 1.26 に上げる（8.4） | `go install` で入れる場合は Go 1.26 以上が必要になる。配布バイナリには影響しない |
| 確認プロンプト | huh をやめ、bubbletea で書いた確認プロンプトにする（8.4） | 選択肢と答えの意味は v0.1.0 と同じ。操作方法が変わる |
| edit の削除 | `dots edit` を削除する（1.2） | `dots edit` は使えなくなる（別名も残さない）。`$EDITOR dots.toml` などで直接開く |
| dots.toml の `[general]` | 新しいトップレベルのテーブル `[general]` を加える。v0.2.0 では `fold_unchanged`、`fold_unchanged_keep`、`source_display` を置く（8.5、8.6） | `[general]` を書いた dots.toml は、v0.1.0 の dots では未知のトップレベルキーとしてエラーになる |
| apply の折りたたみ | 変化のなかった項目（`✓`）が多いと、先頭の数件だけ表示して残りを `… N unchanged` の1行にまとめる。既定で有効。`dots apply --verbose` で全件を表示する（8.5） | 表示の変更 |
| apply の source 表示 | `[general]` の `source_display`（`off`、`simple`、`verbose`）で、apply と apply --dry-run の行に付ける `← source` を切り替える。既定の `simple` は v0.2.0 の 8.3 の動作（8.6） | 新規。既定値では動作は変わらない |
| apply の要約の語 | `already ok` を `unchanged` に変える（apply、apply --dry-run。8.5） | 表示の変更。出力を解析しているスクリプトには影響する |
| 項目のまとまりの間の空行 | apply と status で、`✓` の行と他の行の間に入れていた空行をなくす（8.1、8.2） | 表示の変更 |
| apply --dry-run の空の設定 | dots.toml にリンクが1件もないとき、`apply --dry-run` も `apply` と同じく `Nothing to do: dots.toml defines no links.` の1行だけを表示する（8.2） | 表示の変更 |
| 新コマンド | clean（7章） | - |

## 3. CLIコマンド

v0.1.0 のコマンドに次のコマンドとオプションを追加する。v0.1.0 の `dots edit` は削除する（1.2）。

| コマンド | 動作 |
| --- | --- |
| dots clean | 設定にはないが dots が作ったリンク（stale）を外す。実行前に確認する。 |
| dots clean --no-confirm | 確認せずに外す。 |
| dots clean --dry-run | 外すものと触らないものを表示する。 |
| dots apply --verbose | 変化のなかった項目を折りたたまずに全件表示する（8.5）。 |

すべてのコマンドに共通のオプションとして `--repo <dir>` を加える。リポジトリルートを指定する（4.1）。短い形は設けない。init では使えない。

`dots --help` の表示は次のとおりとする。グループ分けは Issue #1 のコメントにある例に合わせ、v0.2.0 で存在するコマンドだけを並べる。

```
$ dots --help

A brief, declarative, flexible dotfiles manager.

Usage:
  dots <command> [options]

Core:
  apply       Apply dotfiles
  status      Show the status of managed files
  clean       Remove links that are no longer in dots.toml

Usability:
  init        Initialize a dotfiles repository
  restore     Restore files from backups

Options:
  --repo <dir>    Use <dir> as the repository
  -h, --help      Show help
  -v, --version   Show version

Use "dots <command> --help" for more information.
```

## 4. リポジトリの解決

v0.1.0 では、doctor（v0.2.0 で status に改名）、apply、restore、edit（v0.2.0 で削除）はカレントディレクトリから親へ dots.toml を探し、見つからなければエラーにしていた。v0.2.0 では、どこにいても dots を使えるように、リポジトリの場所をファイルに記録し、見つからないときはそれを使う。あわせて、リポジトリを明示的に指定する方法を2つ加える。

### 4.1 解決の順序

対象コマンドは status、apply、clean、restore とする。init は対象外で、v0.1.0 と同じくカレントディレクトリに dots.toml を作る。

1. `--repo <dir>` オプションがあれば、`<dir>` をリポジトリルートとする。`<dir>` はカレントディレクトリを基準に解決する。`<dir>` の直下に dots.toml がなければエラーにする。親ディレクトリは探さない。
2. 環境変数 `DOTS_REPO` が空でなければ、その値をリポジトリルートとする。絶対パスでなければエラーにする。直下に dots.toml がなければエラーにする。
3. カレントディレクトリから親ディレクトリへ順に dots.toml を探す（v0.1.0 と同じ）。見つかればそのディレクトリをリポジトリルートとする。
4. 見つからない場合、リポジトリの記録（4.2）を読む。記録があり、そのディレクトリに dots.toml があれば、そこをリポジトリルートとする。
5. どれでも見つからない場合はエラーにする。記録があったのに dots.toml がない場合は、そのパスをエラーに含める（例: `dots: no dots.toml in the recorded repository ~/dotfiles`）。記録もない場合は、dots.toml があるディレクトリで実行するか `--repo <dir>` を指定すること、新しく作る場合は `dots init` を案内する。

1と2は利用者が明示した指定なので、失敗しても次の方法へは進まない。3を4より優先するのは、v0.1.0 の動作を変えないためである。

`--repo` はリポジトリを指定するだけで、カレントディレクトリは変えない。コマンドライン引数の相対パス（restore の `<archive>`）は、`--repo` があっても実際のカレントディレクトリを基準に解決する。ファイル名は dots.toml に固定し、設定ファイルのパスを直接指定するオプションは設けない。

結果表示の先頭にあるリポジトリルートの表示（v0.1.0 の8章）には、2または4で決めた場合だけ出所を付記する（例: `Repository: ~/dotfiles (recorded)`、`Repository: ~/dotfiles (DOTS_REPO)`）。

### 4.2 リポジトリの記録

`dots status` や `dots clean` などをどこからでも使えるように、普段使うリポジトリルートの場所をファイルに保存する。バックアップ（v0.1.0 の7.1）と同じデータディレクトリに置く。

- Linux、FreeBSD、OpenBSDなど: 有効な XDG_DATA_HOME/dots/repository。XDG_DATA_HOME が未設定または空なら ~/.local/share/dots/repository。設定されているが絶対パスでない場合はエラーにする。
- macOS: ~/Library/Application Support/dots/repository
- Windows: %LOCALAPPDATA%\dots\repository

中身は、リポジトリルートの絶対パスを UTF-8 で1行だけ書いたテキストとする（末尾の改行は任意）。利用者が手で書き換えてもよい。空、複数行、絶対パスでない場合は、記録が壊れているとして警告を表示し、記録がないものとして扱う。書き込みは一時ファイルを経由して rename する。

記録を書くのは `dots apply` だけとする。`--dry-run` でなく、検証エラーなしに処理を終えた場合に書く。部分適用で終わった場合も書く。1〜4のどの方法で決めたリポジトリでも書く。

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

#### 実装上の置き場所

- 既存の `internal/state` はリンクの状態の評価（v0.1.0 の6章）を持つので、記録ファイルの読み書きは別パッケージ `internal/statefile` に置く。
- 保存先の解決は、`internal/backup/backup.go` の `DefaultDir` と同じ OS ごとの switch になる。そのため `internal/platform` に共通関数（例: `AppDir(env, kind)`）を切り出し、backup と statefile の両方から使う。backup の保存先（`XDG_DATA_HOME`、なければ `~/.local/share`）は変えない。

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

読み込みは未知のキーを無視する。Issue #13（copy モード、v0.3.0）を入れるときは、`version` を上げずに別のキー（例: `copies`）を足せるようにするためである。#13 の議論では「symlink は Readlink で判別できるので記録は不要、copy だけハッシュを記録する」としたが、これはドリフト判定の話である。設定から消したルールのリンクを見つける（stale 検出）には symlink の記録が要るので、v0.2.0 の state は `links` を持つ。

### 5.3 記録と削除

- apply は、作成したリンクと、すでに ValidLink だったリンクを記録する。v0.1.0 で作ったリンクも、v0.2.0 で apply すれば記録される。
- 同じ target の記録はひとつだけ持つ。新しく記録するときは上書きする。
- apply は、記録の target が存在しなくなっていれば、その記録を消す。
- clean は、外したリンクと、StaleChanged・StaleGone の記録を消す（7.2）。
- status と各コマンドの --dry-run は state を変更しない。

書き込みは、同じディレクトリの一時ファイルに書き終えてから最終名へ rename する。dots を同時に複数実行した場合の排他制御は行わない。後から書いた方の内容が残る。

### 5.4 stale の判定

stale は「現在の設定にはないが、dots が作ったリンク」である。次の条件をすべて満たす記録を stale とする。

1. 記録の repository が、今回使っているリポジトリルートと同じ。
2. 記録の target が、現在の設定から展開した配置先（上書きされたルールを含む）に含まれない。

stale の記録は、target の現在の状態で次のように分ける。判定には Lstat と Readlink を使い、リンク先はたどらない。

| 状態 | 条件 | dots clean での動作 |
| --- | --- | --- |
| StaleLink | target がシンボリックリンクで、リンク文字列が記録の link と一致する | リンクを削除し、記録を消す |
| StaleChanged | target がシンボリックリンクだがリンク文字列が違う、または通常ファイルやディレクトリになっている | 触らない。記録を消し、報告する |
| StaleGone | target が存在しない | 記録を消す（表示はしない） |

リンク文字列だけで比較するのは、リポジトリを移動してリンク切れになったリンクも、dots が作ったものとして扱うためである。Windows では、比較の前に両方の / を \ にそろえる。

配置先のディレクトリを走査して、リポジトリ内を指すリンクを探す方法は採らない。削除されたルールの配置先がどこにあったかは設定から分からず、走査する範囲を決められないためである。

別のリポジトリで作った記録は stale として扱わない。リポジトリを別の場所へ移動した場合、移動前の記録は stale として検出できない。v0.2.0 では次の2つを制限として README に書く。

- v0.1.0 で作り、v0.2.0 で一度も apply しないままルールを消したリンクは、記録がないので検出できない。
- リポジトリを移動すると、移動前の記録は stale として検出できない。

### 5.5 stateが読めない場合

- ファイルがない: 記録がないものとして扱う。stale は検出されない。エラーにも警告にもしない。
- JSON として読めない、必須キーがない: 読めなかったことを警告として表示し、記録がないものとして扱う。status は `! error` として報告する（終了コード1）。apply は書き込む前に、壊れたファイルを `state.json.broken-<UTC日時>` に rename して残し、新しい state を作る。退避先は表示する。clean は何も削除せずエラー終了する。
- version が 1 より大きい: 新しい dots で書かれたものとみなし、書き換えない。読み取り専用のコマンドは警告を出して記録なしとして続ける。state を書くコマンド（apply、clean）は変更前にエラー終了する。

## 6. 共通の約束

この文書で追加するコマンドは、次を v0.1.0 と共通にする。

- パスの規則（v0.1.0 の4章）。設定ファイルに書くパスは / 区切りで、絶対パスを禁止する。
- 結果表示の形式と記号（v0.1.0 の8章、本書の9章）。先頭にリポジトリルートを表示する。
- 終了コード（v0.1.0 の8章）。0 は成功、1 は失敗または問題の検出、2 は使い方の誤り、130 はユーザーによる中断。
- --dry-run はファイルシステム、state、リポジトリの記録を変更しない。dots.toml はどのコマンドも書き換えない（init が新しく作る場合を除く）。

## 7. dots clean

### 7.1 使い方

```
dots clean
dots clean --no-confirm
dots clean --dry-run
```

- stale（5.4）を片付ける。対象は stale だけで、現在の設定にある配置先には触らない。
- 引数は取らない。引数を渡した場合は使い方の誤り（終了コード2）とする。
- dots.toml は変えない。ルールはすでに dots.toml から消えている。

ある配置先の管理をやめるときは、dots.toml からそのルールを消す（auto なら ignore に加える）。すると、そのリンクは stale になり、`dots clean` で外せる。dots の利用をやめるときも同じで、dots.toml のルールをすべて消してから `dots clean` を実行すれば、記録のあるリンクはすべて外れる。

現在の設定のリンクを外すオプション（`--all` など）は設けない。

### 7.2 リンクの外し方

stale の記録ごとに、5.4の状態で判定する。

| 状態 | リンク | state の記録 |
| --- | --- | --- |
| StaleLink | 削除する | 消す |
| StaleChanged | 触らない（`! skipped`） | 消す |
| StaleGone | 何もしない（表示はしない） | 消す |

- 削除するのはシンボリックリンクそのものだけである。source 側のファイル、リンク先、通常ファイル、ディレクトリは削除しない。
- 削除の直前にもう一度 Lstat と Readlink で状態を確かめる。StaleLink のままなら削除する。存在しなくなっていれば、望む状態になっているので削除したものとして扱う。それ以外に変わっていれば削除せず `! skipped` とし、記録を消す。
- 削除に失敗した場合は `! failed` とし、記録を残す。終了コードは1とする。
- apply が作った親ディレクトリは、空になっても削除しない。
- バックアップからの復元は行わない。

`! skipped` は、dots が作ったリンクがそこにない、という望む状態にすでになっているので、エラーとは扱わない。

state が読めない場合は5.5に従う。壊れている場合も、新しい version の場合も、何も削除せずエラー終了する。state の書き込みに失敗した場合は、外したリンクは戻さず、警告を表示して終了コード1とする。

### 7.3 確認

- 外すリンク（StaleLink）があれば、外す対象と触らない対象を一覧したうえで「Yes / No」を確認する。一括で外すと、stale の判定に誤りがあったときに影響が広いためである。初期カーソルは No とする。
- No なら何も変更せず、`Nothing was changed.` を表示して終了コード0で終える。中断は終了コード130とする。
- `--no-confirm` で確認を省略できる。TTY がなく `--no-confirm` もない場合は、一覧を表示したうえで変更前にエラー終了する。
- 外すリンクがなく、StaleChanged と StaleGone の記録を消すだけの場合は確認しない。リンクには触らないためである。
- stale がひとつもなければ `No stale links.` を表示して終了コード0で終える。

### 7.4 表示

```
Repository: ~/dotfiles

  - stale ~/.config/tmux  ← config/tmux
  ! skipped ~/.zshrc
    changed since dots created it; dots will not remove it

1 removed, 1 skipped
```

- 削除は新しい記号 `-` で示す（9章）。`-` の行には、state に記録した source を `←` の後に表示する（8.3）。
- 削除の直前に状態が変わっていた項目は `! skipped` とし、詳細に `changed since it was checked; left untouched` を表示する。削除に失敗した項目は `! failed` とし、詳細にエラーを表示する。
- 確認を出した場合、一覧は確認の前に表示済みなので、確認の後は一覧と結果が違う項目（`! skipped` に変わったもの、`! failed`）と要約だけを表示する。
- `--dry-run` では一覧の後に `1 to remove, 1 to skip` と `dry run: nothing was changed` を表示する。

### 7.5 v0.2.0 で決めたこと

草案の Issue 4 では `dots unlink` を予定し、「設定からルールを消すか、バックアップを戻すかも、このコマンドに含めるか決める」としていた。v0.2.0 では次のとおりとする。

- `dots unlink` は設けない。stale を片付ける `dots clean` だけを加える。
- dots は dots.toml を書き換えない。dots.toml は利用者が書く宣言で、dots はそれを読んで適用するだけにする。配置先の管理をやめることも、dots.toml からルールを消すという宣言の変更で表し、残ったリンクの片付けを `dots clean` が受け持つ。`dots unlink <target>` のように dots が dots.toml を書き換えると、コメントや書式を残したままテキストを書き換える処理と、上書きされたルールや OS ごとのルールの扱いが必要になり、得られる便利さに比べて複雑になりすぎる。
- バックアップの復元は含めない。復元は `dots restore` で行う。

## 8. 既存コマンドの変更

### 8.1 status

status は、v0.1.0 の doctor を改名したコマンドで、設定と管理しているファイルの状態を表示する。v0.1.0 の診断に加えて stale を表示する。`dots doctor` は削除し、別名も残さない。

#### 改名の理由

Issue #1 で予定していた `status`（管理しているファイルを状態つきで一覧する）と `doctor`（設定と配置先の診断）は、どちらか一方で足りると分かった。そこで1つにまとめ、名前は `status` とする。

#### 既定の出力と --verbose

既定では、各行に記号と配置先だけを表示する。`!` の行には、記号と配置先の間に短い状態の語（`wrong link`、`conflict` など）を置き、直すために必要な詳細（`current link:` など）を次の行に付ける。`← source` と出所（`auto[0]`、`dots."~/.vimrc"` など）は表示しない。

```
$ dots status
Repository: ~/dotfiles

  ✓ ~/.vimrc
  × ~/.config/tmux
  ! wrong link ~/.config/nvim
    current link: ../../dotfiles/config/nvim

1 ok, 1 missing, 1 wrong link
Not OK: run `dots apply` to fix.
```

`--verbose` を付けると、`← source` と出所を、source が分かる全項目に表示する。出所は薄い色で、source の後ろに2スペース空けて置く。`←` の位置は、apply（8.3）と同じ規則で全項目をそろえる。表示は apply の `source_display = "verbose"`（8.6）と同じ処理で作る。status は `--verbose` なしを `off`、`--verbose` ありを `verbose` として扱い、`[general]` の `source_display` には従わない。

```
$ dots status --verbose
Repository: ~/dotfiles

  ✓ ~/.vimrc                   ← vimrc  dots."~/.vimrc"
  × ~/.config/tmux             ← config/tmux  auto[0]
  ! wrong link ~/.config/nvim  ← nvim  dots."~/.config/nvim"
    current link: ../../dotfiles/config/nvim

1 ok, 1 missing, 1 wrong link
Not OK: run `dots apply` to fix.
```

- 項目の行は `✓` → `×` → `!` の順にまとめて表示する。まとまりの間に空行は入れない。同じ記号の中では元の順序を保ち、詳細の行は各項目の直下に置く。apply（8.2）と同じ規則である。
- 要約の行には、上書きされたルールの件数（`N overridden rule(s)`）を含めない。status は上書きされたルールの一覧（`Overridden rules:`）を `--verbose` でも表示しない。この一覧は `dots apply --dry-run` で確認する。
- 状態の語と詳細は、既定でも `--verbose` でも同じものを表示する。問題のある行には、直すために必要な詳細を既定の出力でも省略しない。
- 終了コードは v0.1.0 の doctor から変えない。`--verbose` の有無で終了コードは変わらない。

stale は次のように表示する（既定の出力）。

```
Repository: ~/dotfiles

  ! stale ~/.config/tmux
    not in dots.toml; created by dots on 2026-10-01
  ! stale ~/.zshrc
    changed since dots created it; dots will not remove it

Run "dots clean" to remove stale links.
```

- StaleLink は `! stale` とする。`--verbose` では、記録の source を `←` の後に表示する。`←` は他の行と同じ列にそろえる（8.3）。
- StaleChanged は `! stale` とし、詳細に変更されていることを表示する。dots clean では削除されないことも示す。`--verbose` では、StaleLink と同じく記録の source を表示する。
- StaleGone は表示しない。status は state を書き換えないので、記録はそのまま残る。
- StaleLink と StaleChanged があれば、status の終了コードは1とする。`!` は要対応を表すという v0.1.0 の8章の約束に合わせる。state が壊れている場合（5.5）も1とする。

### 8.2 apply

- apply は 5.3 のとおり state を更新する。
- stale は削除しない。stale が1件以上あれば、要約の後に件数と `Run "dots clean" to remove them.` を表示する。
- state の書き込みに失敗しても、作ったリンクは戻さない。警告を表示し、終了コードは1とする。
- 項目の行は `✓` → `+` → `~` → `!` の順にまとめて表示する。まとまりの間に空行は入れない。同じ記号の中では元の順序を保ち、`backup:` などの補足行は各項目の直下に置く。`--dry-run` の計画も同じ順にする。
- dots.toml にリンクが1件もないときは、`--dry-run` の有無にかかわらず `Nothing to do: dots.toml defines no links.` の1行だけを表示し、終了コード0で終わる。
- 変化のなかった項目（`✓`）が多い場合は折りたたむ。要約の `already ok` は `unchanged` に変える（8.5）。

### 8.3 変更した項目の source を表示する（#9）

#### 現象

apply の結果表示には配置先だけが出る。どの source から来たリンクかは、`dots status --verbose` を実行しないと分からない。

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
  + ~/.config/wezterm  ← config/wezterm

1 created, 3 unchanged
```

- 既定（`source_display = "simple"`、8.6）の対象は「変更した」項目とする。apply と apply --dry-run の `+`（作成）と `~`（置換）、clean の `-`（削除）の行に付ける。`✓` と `!` の行には付けない。`[general]` の `source_display` で、付けない（`off`）ことや、全項目に出所つきで付ける（`verbose`）ことも選べる（8.6）。clean の行は設定に従わず、常にこの規則とする。
- source はリポジトリルートからの相対パスで、/ 区切りで表示する（v0.1.0 の8章の表示規則と同じ）。stale の項目は state に記録した source を使う。
- `linked to` のような語は使わず、`←` だけを使う。
- source は薄い色（dim）で表示する。色なしの場合（`NO_COLOR`、TTY でない場合）も `← source` の文字は出す。
- `←` の位置は、その一覧で `←` を付ける行のうち最も長い「記号＋配置先」に合わせて揃える。幅は表示幅（全角文字は2）で数える。揃えた結果が端末幅を超える場合は、揃えずに配置先の後ろに空白2つで続ける。
- status の source の表示（v0.1.0 の8章の `-> source`）も `←` に変える。矢印の向きと意味を、すべてのコマンドで「配置先 ← source」にそろえるためである。status は既定では source と出所を表示せず、`--verbose` を付けたときだけ全項目に `← source` と出所を表示する（8.1）。変更した項目だけに付ける規則は、apply、clean に適用する。
- apply、apply --dry-run、status、clean は、同じ表示処理（ui の `SourceMode`）で行を作る。そのため `←` の列揃えは status --verbose と stale の行にも同じように働く。

#### やること

- [ ] ui の項目行に、変更した項目だけ source を付ける処理を追加する（apply、apply --dry-run）。
- [ ] `←` の列揃えと、端末幅を超える場合の扱いを実装する。この処理は ui パッケージの共通関数にする。status の stale、clean の `-` の行でも同じ揃え方を使うためである。
- [ ] clean の表示を同じ規則で実装する。
- [ ] status の `-> source` を `← source` に変え、既定では source と出所を表示せず、`--verbose` のときだけ表示する。上書きされたルールの一覧を `--verbose` のときだけにし、要約の上書き件数をなくす。
- [ ] README の出力例を更新する。
- [ ] 単体テスト: 変更した項目だけに付くこと、列揃え、全角文字を含むパス、色なしの出力。

#### 完了の条件

- apply の結果で、`+` と `~` の行にだけ `← <source>` が出る。
- 色なしの出力に ANSI エスケープが含まれない。

### 8.4 競合の確認プロンプトの修正（#11）

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
- 確認はすべてこの部品で作る。apply の競合（`Confirm`）、restore（`ConfirmRestore`）、v0.2.0 で増える clean の Yes／No（7.3）である。確認ごとに個別の画面は書かない。

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

v0.2.0 の確認は次の3つになる。

| 確認 | 選択肢 | チェックボックス | 初期カーソル |
| --- | --- | --- | --- |
| apply の競合（v0.1.0 の6章） | Yes – back up and replace / No – skip | Process the rest of the files similarly | No |
| restore の既存ターゲット（v0.1.0 の7.2） | Back up current content and restore / Skip | なし | Skip |
| dots clean（7.3） | Yes – remove them / No – keep them | なし | No |

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
- [ ] v0.2.0 の新しい確認（7.3）をこの部品で作る。
- [ ] tty-e2e の expect シナリオを更新する。下キーでチェックボックスへ移れること、Space で切り替えて Yes to All になること、Ctrl-C で130になることを確認する。
- [ ] #4 の再現手順（`printf '\r' | script -qec "dots apply" /dev/null`）でプロンプトが止まらないことを確認する。

#### 完了の条件

- 競合の確認で、上下キーだけでチェックボックスへ移動でき、Space で切り替えられる。
- チェックボックスをオンにして Yes を選ぶと、以降の競合が確認なしでバックアップ付き置換される。No も同様にスキップされる。
- Enter で確定せずに終わった確認は、すべて終了コード130になる。
- #4 の再現手順でプロンプトが止まらない。
- Linux と macOS の tty-e2e が通る。Windows の CI で、ビルドと部品の単体テストが通る。

### 8.5 変化のなかった項目の折りたたみ（#10）

#### 現象

apply の結果には、変化のなかった項目（`✓`）がすべて並ぶ。管理する項目が増えると、作成や置換した行が `✓` の行に埋もれる。

#### 設定

dots.toml に新しいトップレベルのテーブル `[general]` を加え、次の3つのキーを置く。`source_display` は 8.6 で扱う。

```toml
[general]
fold_unchanged = true
fold_unchanged_keep = 5    # 折りたたむときに残す ✓ の行数
source_display = "simple"  # off | simple | verbose（8.6）
```

| キー | 型 | 既定値 | 意味 |
| --- | --- | --- | --- |
| fold_unchanged | 真偽値 | true | 変化のなかった項目を折りたたむかどうか |
| fold_unchanged_keep | 0 以上の整数 | 5 | 折りたたむときに残す `✓` の行数 |
| source_display | 文字列（`"off"`、`"simple"`、`"verbose"`） | `"simple"` | apply と apply --dry-run の行に `← source` を付ける範囲（8.6） |

- `[general]` も、その中の各キーも省略できる。省略したキーには既定値を使う。
- `[general]` の中の未知のキー、型の違い、`fold_unchanged_keep` の負の数、`source_display` の未知の値はエラーにする。v0.1.0 の dots.toml の検証と同じく、打ち間違いをそのまま通さない。
- `fold_unchanged = false` のときも、`fold_unchanged_keep` の値は検証する。
- `[general]` は OS ごとに分けない。`[general.linux]` のような書き方は v0.2.0 では設けず、エラーにする。

#### 表示

`fold_unchanged_keep = 2` の場合の例を示す。

```text
Repository: ~/dotfiles

  ✓ ~/.config/fish
  ✓ ~/.config/nvim
  … 47 unchanged
  + ~/.config/wezterm  ← config/wezterm

1 created, 49 unchanged
```

- 対象は apply と apply --dry-run の `✓` の行とする。status と clean の行は折りたたまない。
- 変化のなかった項目の件数を U、`fold_unchanged_keep` を K とする。U が K + 1 以下なら全件を表示する。U が K + 1 を超えたら、先頭から K 件の `✓` を表示し、残りの U − K 件を `… N unchanged` の1行にまとめる（N = U − K）。
- U が K + 1 のときに折りたたまないのは、1行を `…` の1行に置き換えても行数が減らないためである。
- K = 0 なら `✓` の行を1行も表示せず、`… N unchanged` だけを表示する（U が2件以上の場合）。
- 「先頭」は、8.2 で並べた後の `✓` のまとまりの中の順序（元の順序）とする。
- `… N unchanged` の行は `✓` のまとまりの最後に置く。2スペースの字下げの後に記号の位置で `…`（U+2026）を出し、空白1つの後に `N unchanged` を続ける。薄い色（dim）で表示する。色なしの場合も同じ文字を出す。
- `… N unchanged` の行には `←` を付けず、`←` の列揃え（8.3）の計算にも含めない。
- `fold_unchanged = false` のときは全件を表示し、`fold_unchanged_keep` は使わない。
- `dots apply --verbose`（`--dry-run` と併用できる）を付けると、設定にかかわらず全件を表示する。apply の `--verbose` は折りたたみを無効にするだけで、`← source` を付ける行（8.3）は変えない。`source_display` も変えない（8.6）。
- 要約では、apply と apply --dry-run の `already ok` を `unchanged` に変える。件数は、折りたたんだかどうかにかかわらず、変化のなかった項目の全件数とする。
- すべての項目に変化がない場合も同じ規則で折りたたみ、その後に `Everything is up to date.` を表示する。

#### v0.2.0 で決めたこと

- 折りたたみは既定で有効にする。apply の結果で知りたいのは何が変わったかで、`✓` の一覧は項目が増えるほど邪魔になるためである。全件を見たいときは `--verbose` を付けるか、`fold_unchanged = false` を書く。
- 設定は dots.toml の `[general]` に置く。`[general]` は今後ほかの設定も入れるテーブルとして新しく作る。
- 残す行数のキー名は `fold_unchanged_keep` とする。`fold_unchanged_after` は「5秒後」のように時間と読まれやすい。`fold_unchanged_threshold` は折りたたんだ後に何件残るかが分からない。`fold_unchanged_max` は「最大5件を折りたたむ」とも読める。`keep` なら「折りたたむときに5件残す」としか読めず、0 のときに1件も残さない意味ともそのまま合う。
- 有効・無効と残す行数は別のキーにする。1つの整数にまとめると、0 を「全部折りたたむ」に使っているので、無効を表すために -1 のような特別な値が要る。
- 残す `✓` は、意味のある選び方がないので先頭から K 件とする。

#### やること

- [x] config に `[general]` の読み込みと検証を加える。トップレベルの未知のキーのエラーメッセージで、使えるキーに `general` を加える。`source_display` の読み込みと検証も含める（8.6）。
- [x] ui の項目一覧に折りたたみを加える（apply、apply --dry-run）。
- [x] apply の要約の `already ok` を `unchanged` に変える。
- [x] apply と status で、`✓` のまとまりと他の行の間の空行をなくす（8.1、8.2）。
- [x] `dots apply --verbose` を加える。
- [x] README の出力例と dots.toml の説明を更新する。
- [ ] 単体テスト: `[general]` の省略・既定値・型の違い・負の数・未知のキー、U が K 以下・K + 1・K + 2 以上の場合、K = 0、`fold_unchanged = false`、`--verbose`、色なしの出力、`←` の列揃えに `…` の行が影響しないこと。

#### 完了の条件

- `[general]` を書かない dots.toml で、変化のなかった項目が7件以上あると、先頭の5件と `… N unchanged` が表示される。
- `fold_unchanged = false` または `--verbose` で、全件が表示される。
- apply と apply --dry-run の要約に `already ok` が出ず、`unchanged` が出る。
- apply と status の出力で、`✓` の行と他の行の間に空行が入らない。

### 8.6 source の表示の設定

#### 設定

`[general]` の `source_display` で、`dots apply` と `dots apply --dry-run` の行に `← source` を付ける範囲を選ぶ。

| 値 | 動作 |
| --- | --- |
| `"off"` | `← source` を付けない。 |
| `"simple"`（既定） | 作成（`+`）と置換（`~`）の行にだけ `← source` を付ける。8.3 の動作である。 |
| `"verbose"` | source が分かる全項目に `← source  出所` を付ける。`✓` の行と `!` の行も含む。`dots status --verbose`（8.1）と同じ表示である。 |

`"simple"` の例を示す。

```text
  + ~/.vimrc  ← vimrc
  + ~/.zshrc  ← zsh/zshrc
```

`"verbose"` では、同じ行が `← zsh/zshrc  auto[0]` のように、source の後ろに空白2つで出所が付くになる。

- 値が文字列でないとき、`"off"`、`"simple"`、`"verbose"` のどれでもないときは、他の `[general]` のキーと同じくエラーにする。
- `dots apply --verbose` は `source_display` を変えない。`--verbose` は折りたたみ（8.5）を無効にするだけである。
- 対象は apply と apply --dry-run である。status は `source_display` に従わず、`--verbose` の有無で決まる（8.1）。clean の行は常に `-` の行にだけ `← source` を付ける（7.4、8.3）。
- `fold_unchanged` による折りたたみとは独立している。`… N unchanged` の行には `←` を付けない（8.5）。

#### v0.2.0 で決めたこと

- キー名は `source_display` とする。`show_source` のような `show_*` は真偽値に読めるが、この設定は3つの値から選ぶ。名詞句の `source_display` なら表示の方式を選ぶ設定として読める。また、将来 status にも広げる場合でも、名前を変えずに済む。このキー名は Gemini に相談して決めた。
- 値は `off`、`simple`、`verbose` とする。`verbose` は status の `--verbose` と同じ表示で、2つのコマンドで同じ処理を使う。
- apply の `--verbose` と `source_display` は結び付けない。`--verbose` は一度だけ全件を見るための指定で、dots.toml の設定を上書きするものではないためである。
- 既定は `simple` とし、設定を書かない dots.toml では 8.3 の動作を変えない。

#### やること

- [x] config に `source_display` の読み込みと検証を加える。
- [x] apply と status が同じ表示処理（ui の `SourceMode`）を使うようにし、status --verbose の `←` を揃える。
- [x] status --verbose から `Overridden rules:` を外す。
- [x] README の dots.toml の説明を更新する。
- [ ] 単体テスト: `source_display` の省略・3つの値・未知の値・型の違い、`off` と `verbose` の出力、status --verbose の列揃えと stale の source、色なしの出力。

#### 完了の条件

- `source_display` を書かない dots.toml で、apply は `+` と `~` の行にだけ `← source` を表示する。
- `"off"` では `← source` が出ない。`"verbose"` では全項目に出所つきで出る。
- `dots status --verbose` に `Overridden rules:` が出ず、`←` が1つの列にそろう。


## 9. 表示と記号

v0.1.0 の8章の記号に次を加える。

| 記号 | 色 | 意味 | 使うコマンド |
| --- | --- | --- | --- |
| `-` | 赤 | 削除（リンクを外す）。短い語 `stale` を付ける | clean |
| `!` | 黄 | 問題あり。新しい短い語として `stale`（status）、`skipped` と `failed`（clean）を加える | status、clean |
| `…` | 薄い色 | 折りたたんだ変化のなかった項目。`… N unchanged` の形で使う（8.5） | apply |

- 項目行の形（2スペースの字下げ、記号、パス、詳細は4スペースの字下げ）は v0.1.0 と同じとする。
- 色なしの扱い、エラーメッセージ中のパスの省略も v0.1.0 と同じとする。

## 10. テスト方針

v0.1.0 の9章の方針を続ける。memfs で次を単体テストする。

- 4章のリポジトリ解決（`--repo`、`DOTS_REPO`、カレントディレクトリ、記録の優先順位、指定先に dots.toml がない場合、記録が壊れている場合）と、記録を書く条件（init は記録しない、など）。
- state の読み書き、壊れたファイルの退避、新しい version の拒否、記録と削除の規則。`internal/platform` の `AppDir` の OS ごとの解決（5.1）。
- stale の判定（StaleLink、StaleChanged、StaleGone、別リポジトリの記録の除外）。
- clean の状態ごとの動作、削除直前の再確認、削除に失敗したときに記録を残すこと、確認の有無（TTY、`--no-confirm`、外すリンクがない場合）、引数を渡したときの使い方の誤り。

CI の実バイナリE2E（smoke.sh）に次のシナリオを加え、5つのOSで実行する。

1. dots.toml からルールを消し、`dots status` が stale を表示し、`dots clean` で消え、`dots status` が stale を表示しなくなること。
2. リポジトリで `dots apply` した後、リポジトリ外のディレクトリで `dots status` が記録したリポジトリを使い、`--repo` と `DOTS_REPO` がそれより優先されること。

clean の確認プロンプト（Yes／No、Ctrl-C で終了コード130）は、v0.1.0 と同じく Linux と macOS で expect を使って確認する。

### 実装の順序（#9、#11 + #4、#6）

議論なしで着手できる #9、#11 + #4、#6 は、次の順に進める。

```text
Wave 1（並行）
  A: #11 + #4  確認プロンプトの部品・Go 1.26・huh の削除（8.4）
  B: #9        ← source 表示（apply、apply --dry-run、status --verbose）（8.3）
  C: #6 前半   AppDir の共通化・statefile パッケージ・apply での記録・stale 判定（5章、8.2）

Wave 2（A、B、C をマージした後）
  D: #6 後半   status の stale 表示・apply の案内・dots clean（7章、8.1、8.2）
  E: #10       [general] の読み込み・apply の折りたたみ・apply --verbose・空行の削除（8.1、8.2、8.5）
```

D は、A の確認プロンプトの部品（clean の Yes／No）と、B の `←` 表示（status の stale、clean の `-` の行）を使うので、後に回す。E は、B で ui パッケージの項目一覧（列揃え）が変わった後に手を入れるので、後に回す。D と E は並行して進められる。

## 11. 未決事項と次のバージョンへの申し送り

- 複数のdotfilesリポジトリを使い分ける場合の扱い。v0.2.0 では state の記録はリポジトリごとに分けて持ち、リポジトリの記録（4.2）は1つだけである。一時的な切り替えは `--repo` と `DOTS_REPO` で行う。
- リポジトリを移動した後に、移動前の記録を新しい場所へ付け替える方法。
- clean 時に source の中身をコピーして残すオプション（dots の利用をやめる場合向け）。
- 共通ルールを特定のOSでだけ無効にする書き方（例: `[dots.darwin]` の `"~/.vimrc" = false`、auto の OS ごとの ignore）。共通ルールを消すと全OSで配置されなくなる。このOSでだけ外したい場合は、今のスキーマでは書けない。
- `dots diff`（Issue #1 のコメントにあるヘルプ例）。
