# v0.2.0 着手分の要点（ドラフト）

`architecture.md` のうち、議論なしで着手できる issue（#9、#11 + #4、#6）の重要なところだけを抜き出したメモ。詳細と根拠は `architecture.md` の該当章を正とする。食い違いがあれば `architecture.md` に合わせる。

## 対象

| Issue | 内容 | 仕様 |
| --- | --- | --- |
| #9 | apply で変更した項目に `← source` を出す。doctor の `->` も `←` に変える | 12.3 |
| #11 + #4 | 確認プロンプトを huh から bubbletea v2 の自前部品に置き換える | 12.4 |
| #6 | dots が作ったリンクを state ファイルに記録し、stale を doctor で報告、`unlink --stale` で消す | 5章、10章（--stale のみ）、12.1、12.2 |

対象外: #13（copy モード、保留中）、#10（折りたたみ、保留中）、#12（親ディレクトリ。現状は `internal/apply/execute.go` の `place` が確認なしで `MkdirAll(0o755)` する。v0.2.0 では変えない）、`dots unlink <target>`、bootstrap、add、cd。

## 作業の分け方と順序

```text
Wave 1（並行）
  A: #11 + #4  確認プロンプト部品・Go 1.26・huh 削除
  B: #9        ← source 表示（apply、apply --dry-run、doctor）
  C: #6 前半   AppDir 共通化・statefile パッケージ・apply での記録・stale 判定

Wave 2（A、B、C をマージした後）
  D: #6 後半   doctor の stale 表示・apply の案内・dots unlink --stale
```

D は A のプロンプト部品（--stale の Yes/No）と B の `←` 表示を使うので後にする。

## #11 + #4: 確認プロンプト

- `charm.land/bubbletea/v2` の最新版を使う。go.mod の `go` を 1.26.0 に上げる。CI（`.github/workflows/ci.yml`）、README、PKGBUILD、flake.nix の最小 Go も 1.26 にする。
- huh を go.mod から外す。`Confirm` と `ConfirmRestore` を新しい部品で置き換える。
- 部品が持つのは質問文、選択肢（2〜4個）、任意のチェックボックス1つ、ヘルプ1行だけ。絞り込み、スクロール、複数チェックボックス、マウス、テーマは作らない。
- 操作: ↑/↓ と j/k で選択肢とチェックボックスの間を移動。Space でチェックの切り替え（`[ ]` / `[x]`）。Enter で確定。チェックボックス上で Enter を押したら、直前に選んでいた選択肢を答えにする。
- 初期カーソルは何も変更しない選択肢（No / Skip / Cancel）。チェックボックスは常にオフで始める。
- Enter で確定していない終わり方（Ctrl-C、SIGTERM、SIGHUP、入力の終わり）はすべて中断で、終了コード130。確定したかどうかは部品自身が持つ（#2 の再発防止）。SIGHUP の扱いは今の `runForm` の方法を引き継ぐ。
- 確定後は表示を消し、`? ~/.vimrc  Yes – back up and replace (and the rest)` の1行を残す。中断なら `? ~/.vimrc  cancelled`。
- 色なし（`NO_COLOR`）でも `>`、`[ ]`、`[x]` は出す。
- 背景色の問い合わせはしない（#4 はこれで直る）。

完了の条件: 上下キーだけでチェックボックスへ移れる。`printf '\r' | script -qec "dots apply" /dev/null` で止まらない。未確定の終了は130。tty-e2e（`scripts/testing` と CI の expect シナリオ）を更新して通す。

## #9: `← source` 表示

- apply と apply --dry-run の `+`（作成）と `~`（置換）の行にだけ `← <source>` を付ける。`✓` と `!` には付けない。
- source はリポジトリルートからの相対パスで、`/` 区切り。dim で表示する。色なしでも `← source` の文字は出す（ANSI エスケープは出さない）。
- `←` の列は、その一覧で `←` を付ける行のうち最も長い「記号＋配置先」に揃える。幅は表示幅（全角は2）。揃えると端末幅を超える場合は、揃えずに配置先の後ろへ空白2つで続ける。
- doctor の `-> source` を `← source` に変える。doctor は従来どおり全項目に表示する。
- README の出力例を更新する。
- 列揃えの処理は D（doctor の stale、unlink の `-` 行）でも使うので、ui パッケージの共通関数にする。

## #6: state ファイルと stale

### パッケージ

- 既存の `internal/state` はリンク状態の評価（v0.1 仕様 6章）なので、記録ファイルは別パッケージ `internal/statefile` にする。
- 保存先の解決は `internal/backup/backup.go` の `DefaultDir` と同じ switch になるので、`internal/platform` に共通関数（例: `AppDir(env, kind)`）を切り出し、backup もそれを使う。backup の挙動（`XDG_DATA_HOME`、`~/.local/share`）は変えない。

### 保存先

- Linux / BSD: `$XDG_STATE_HOME/dots/state.json`。未設定・空なら `~/.local/state/dots/state.json`。相対パスならエラー。
- macOS: `~/Library/Application Support/dots/state.json`
- Windows: `%LOCALAPPDATA%\dots\state.json`
- ディレクトリとファイルは現在のユーザーだけがアクセスできる権限で作る。

### 形式

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

- links は target の辞書順。同じ target は1件だけ（上書き）。target の比較は v0.1 の 5.4 の正規化（Windows と macOS は大文字・小文字を区別しない）。
- 書き込みは同じディレクトリの一時ファイルに書いてから rename。排他制御はしない。

### 記録の規則

- apply は、作ったリンクとすでに ValidLink だったリンクを記録する。記録の target が存在しなければ記録を消す。
- doctor と --dry-run は state を変更しない。
- state の書き込みに失敗しても作ったリンクは戻さない。警告を出し、終了コード1。

### stale の判定

記録の repository が今のリポジトリルートと同じで、target が現在の設定から展開した配置先に含まれないもの。Lstat と Readlink で分類する（リンク先はたどらない）。

| 状態 | 条件 | unlink --stale |
| --- | --- | --- |
| StaleLink | シンボリックリンクで、リンク文字列が記録と一致（Windows は `/` を `\` にそろえて比較） | リンクを削除し、記録を消す |
| StaleChanged | リンク文字列が違う、または通常ファイルやディレクトリ | 触らない（`! skipped`）。記録を消す |
| StaleGone | 存在しない | 記録を消す（表示しない） |

配置先ディレクトリの走査はしない。

### state が読めない場合

- ファイルがない: 記録なし。警告もしない。
- 壊れている（JSON として読めない、必須キーがない）: 警告して記録なしとして扱う。doctor は `! error`（終了コード1）。apply は `state.json.broken-<UTC日時>` に rename して退避し、新しく作る。退避先を表示する。unlink --stale は何も削除せずエラー終了。
- version が 1 より大きい: 書き換えない。読むだけのコマンドは警告して続ける。書くコマンドは変更前にエラー終了。

### 表示（D）

doctor:

```text
  ! stale ~/.config/tmux  ← config/tmux
    not in dots.toml; created by dots on 2026-10-01
  ! stale ~/.zshrc
    changed since dots created it; dots will not remove it

Run "dots unlink --stale" to remove stale links.
```

- StaleLink か StaleChanged があれば doctor の終了コードは1。
- apply は stale を消さない。1件以上あれば要約の後に件数と `Run "dots unlink --stale" to remove them.` を出す。
- `dots unlink --stale`: 一覧を出して Yes/No で確認（初期カーソル No、A の部品を使う）。`--yes` で確認を省略、`--dry-run` あり。削除の直前に Lstat と Readlink で再確認し、判定時と違えば `! skipped`。削除するのはシンボリックリンクだけ。空になった親ディレクトリは消さない。削除行は `- <target>  ← <source>`。

### #13 との関係

#13 の議論では「symlink は Readlink で判別できるので記録不要、copy だけハッシュを記録」とした。これはドリフト判定の話で、stale 検出（設定から消えたルールのリンク）には symlink の記録が要る。v0.2.0 の state.json は #6 のための `links` を持ち、#13 を入れるときは `version` を上げずに別のキー（例: `copies`）を足せるようにしておく。読み込みは未知のキーを無視する。

### 制限（README に書く）

- v0.1.0 で作り、v0.2.0 で一度も apply する前にルールを消したリンクは検出できない。
- リポジトリを移動すると、移動前の記録は stale として検出できない。

## テスト

- memfs の単体テスト: statefile の読み書き、壊れたファイルの退避、新しい version の拒否、記録と削除の規則、stale の3分類、別リポジトリの記録の除外、AppDir の OS ごとの解決。
- ui: `←` が変更項目だけに付く、列揃え、全角を含むパス、色なしで ANSI なし。
- プロンプト部品: キーメッセージを Update に渡して状態を確かめる（端末は使わない）。未確定の終了が130。
- smoke.sh に追加: dots.toml からルールを消す → doctor が stale を表示 → `dots unlink --stale --yes` で消える → doctor が stale を出さない。
