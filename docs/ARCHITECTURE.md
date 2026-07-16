# Architecture - `sono`

## Vue d'ensemble

`sono` est un gestionnaire de versions Node.js piloté par une interface en terminal (TUI).
Il joue le même rôle que `nvm` / `fnm` / `volta` : toutes les actions se font depuis une interface plein écran directement dans le terminal.

L'outil est un binaire Go unique et autonome.
Sans argument, il lance une TUI interactive ; avec un sous-commande (`sono install 20`, `sono use 20`, `sono ls`), il agit de façon non interactive pour le scripting.
Dans les deux cas il récupère la liste officielle des versions Node.js, permet de télécharger/activer/supprimer une version, et signale les mises à jour disponibles.

### Objectifs

- Installer et gérer plusieurs versions de Node.js sans avoir Node ni npm sur le système.
- Filtrer les versions par LTS / non-LTS.
- Voir d'un coup d'œil ce qui est installé, ce qui est actif, et ce qui est disponible.
- Changer la version active d'une touche, de façon instantanée et réversible, sans droits root.
- Vérifier l'intégrité de chaque téléchargement (SHA256) avant installation.

### Non-objectifs (pour l'instant)

- Gestion par projet (fichier `.nvmrc` par dossier) : hors périmètre initial.
- Support multi-utilisateurs ou installation système globale (`/usr/local`) : `sono` travaille dans le home de l'utilisateur.
- Gestion de npm/pnpm/yarn séparément : ils viennent avec le tarball Node officiel.

## Choix de la stack et justification

| Élément | Choix | Raison |
| --- | --- | --- |
| Langage | Go (stdlib) | Binaire unique, pas de runtime à installer, excellent pour manipuler archives et HTTP. |
| Interface (rendu) | **Bubble Tea** (+ `bubbles`, `lipgloss`) | Framework TUI en Go, architecture à la Elm (`Model` / `Update` / `View`). Adapté aux mises à jour asynchrones (progression de téléchargement en flux), tableaux, spinners. Pas de build front, pas de Node. |
| Dépendances tierces | `github.com/ulikunitz/xz` + la pile Bubble Tea | Toutes en Go pur : le binaire reste unique et autonome. `xz` sert à décompresser les tarballs Node `.tar.xz` (plus légers que `.tar.gz`), la stdlib n'ayant pas de décodeur xz. |

### Pourquoi pas de Node dans la stack

Le but de l'outil est précisément d'installer Node sur une machine qui n'en a pas.
Utiliser un framework d'interface qui exige Node pour être compilé (Svelte, React, etc.) créerait une dépendance circulaire au moment du build.
Bubble Tea étant du Go pur, `sono` se construit et fonctionne sans jamais avoir besoin de Node.

## Structure du projet

```
sono/
  go.mod                     # module sophonie/sono, go 1.26.5
  main.go                    # point d'entrée : config + auto-purge ; dispatch CLI ou lance la TUI
  internal/
    config/                  # chemins ~/.sono, détection OS/arch, réglages
    nodedist/                # client nodejs.org : index.json, SHASUMS256, download
    manager/                 # install / uninstall / set-active (symlink) / list installées
    pkgmgr/                  # pnpm / yarn : versions, install, activate, uninstall
    tui/                     # interface Bubble Tea (modèle racine, onglets Node et PM)
    cli/                     # sous-commandes non interactives (install, use, ls, pm, cache…)
```

Les couches `tui` et `cli` ne font que présenter les paquets du domaine (`config`, `nodedist`, `manager`, `pkgmgr`) ;
ces derniers ignorent tout de l'interface, ce qui a permis de remplacer l'ancien dashboard web par une TUI puis d'ajouter une CLI sans les toucher.
Le binaire final ne dépend d'aucun fichier externe pour tourner.

## Disposition des données sur le système

Tout vit sous `~/.sono/` :

```
~/.sono/
  versions/
    v20.11.0/                # contenu extrait du tarball officiel (bin/, lib/, ...)
    v22.5.1/
  current -> versions/v20.11.0   # symlink vers la version active
  cache/                     # tarballs téléchargés (réutilisables, purgeables)
  index.json                 # cache local de la liste des versions
```

### Mécanisme de la version active

La version active est déterminée par le symlink `~/.sono/current`.
Activer une version consiste uniquement à repointer ce symlink : c'est instantané, atomique et réversible.

L'utilisateur ajoute **une seule fois** `~/.sono/current/bin` à son `PATH` (dans `.bashrc` ou équivalent).
`sono` affiche la ligne exacte à coller.
Après cet ajout, `node` et `npm` pointent toujours vers la version active courante, sans aucune autre manipulation.

Le dashboard distingue deux notions :

- La version **active** selon le symlink `current` (ce que `sono` gère).
- La version réellement résolue sur le `PATH` du shell (ce que le système exécute vraiment).

Un écart entre les deux signale généralement que le `PATH` n'a pas encore été configuré, ou qu'un autre Node est installé ailleurs.

## Source des données Node.js

- Index des versions : `https://nodejs.org/dist/index.json`.
  Chaque entrée contient `version`, `date`, `files`, et surtout `lts` qui vaut `false` (non-LTS) ou le nom de code de la ligne LTS (ex. `"Iron"`).
  Ce champ sert directement de filtre LTS / non-LTS.
- Tarball : `https://nodejs.org/dist/v<X.Y.Z>/node-v<X.Y.Z>-<os>-<arch>.tar.xz`.
- Checksums : `https://nodejs.org/dist/v<X.Y.Z>/SHASUMS256.txt`.

### Détection OS / architecture

`nodedist` traduit `runtime.GOOS` et `runtime.GOARCH` vers la nomenclature Node :

| Go | Node |
| --- | --- |
| `linux` + `amd64` | `linux-x64` |
| `linux` + `arm64` | `linux-arm64` |
| `darwin` + `arm64` | `darwin-arm64` |
| `windows` + `amd64` | `win-x64` |

La cible principale de développement est `linux-x64`.

## Responsabilités des composants

### `config`

- Résout et crée l'arborescence `~/.sono/` (`versions`, `cache`).
- Expose les chemins (racine, dossier versions, symlink current, cache).
- Détecte OS/arch et fournit la chaîne de plateforme Node.
- Lit les flags de démarrage (port, adresse d'écoute).

### `nodedist`

- Récupère et met en cache `index.json`.
- Parse les versions, expose des filtres (LTS, non-LTS, recherche).
- Récupère et parse `SHASUMS256.txt`.
- Télécharge un tarball en streaming, en calculant le SHA256 au vol.

### `manager`

- Liste les versions installées (scan de `~/.sono/versions`).
- Lit la version active (`readlink current`).
- Installe une version : télécharge (via `nodedist`), vérifie le SHA256, extrait le tarball, range dans `versions/`.
- Active une version : repointe le symlink `current` de façon atomique.
- Supprime une version : retire le dossier, et refuse/avertit si c'est la version active.
- Calcule les mises à jour disponibles : compare chaque version installée au dernier patch de sa ligne mineure et au dernier LTS.

### `pkgmgr`

- Récupère la liste des versions stables de pnpm / yarn depuis le registre npm.
- Installe une version (`.tgz` npm, vérifié via l'intégrité SHA512), l'active via des shims, la supprime.

### `tui`

- Modèle racine Bubble Tea avec deux onglets (Node, gestionnaires de paquets) et une barre d'aide.
- Chaque onglet assemble sa vue à partir des paquets du domaine et rend un tableau navigable au clavier.
- Suit l'état des téléchargements en cours (progression) directement dans le modèle.
- Reçoit la progression d'installation en flux via un canal (`tea.Cmd`), sans polling.

### `cli`

- Analyse les sous-commandes et délègue aux paquets du domaine ; renvoie un code de sortie.
- Résout les versions partielles (`20`, `20.11`, `lts`, `latest`) vers une version concrète.
- Sépare résultats (stdout) et progression (stderr) ; la progression de téléchargement ne s'anime que sur un vrai terminal.

## Flux d'installation

1. L'utilisateur sélectionne une version et appuie sur `enter` / `i`.
2. La TUI lance l'installation dans une goroutine qui pousse la progression dans un canal.
3. `nodedist` télécharge le tarball vers `cache/`, en calculant le SHA256 pendant l'écriture.
4. Le SHA256 calculé est comparé à l'entrée correspondante de `SHASUMS256.txt`.
   En cas de non-correspondance, le fichier est supprimé et l'installation échoue explicitement.
5. Le tarball vérifié est extrait dans `versions/v<X.Y.Z>/`.
6. Chaque événement du canal est transformé en message Bubble Tea ; la vue reflète la progression en temps réel, puis l'état final (installé / erreur).

La vérification du checksum est obligatoire et non contournable.

## Concurrence

- Chaque installation tourne dans sa propre goroutine.
- La progression et les erreurs remontent par un canal, transformées en messages par une `tea.Cmd` ; la boucle Bubble Tea sérialise les mises à jour d'état, sans mutex.
- Le repointage du symlink `current` est atomique (création d'un lien temporaire puis `rename`).

## Sécurité

- `sono` ne fait rien écouter sur le réseau : c'est un binaire local, piloté au clavier.
- Vérification SHA256 systématique avant toute extraction.
- Aucune commande shell n'est construite à partir d'entrées utilisateur ; les versions sont validées contre l'index officiel avant usage.
- `sono` n'a jamais besoin de droits root : tout se passe dans le home de l'utilisateur.

## Décisions d'architecture (résumé)

- Bubble Tea (Go pur) pour l'interface : pas de Node au build, binaire unique, mises à jour asynchrones naturelles.
- Séparation nette domaine / présentation : les paquets `config`, `nodedist`, `manager`, `pkgmgr` sont agnostiques de l'UI.
- Stdlib Go, plus `github.com/ulikunitz/xz` pour décompresser les tarballs `.tar.xz` : robustesse et maintenabilité long terme.
- Version active gérée par symlink : instantané, atomique, réversible, sans root.
