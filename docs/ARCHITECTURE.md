# Architecture - `sono`

## Vue d'ensemble

`sono` est un gestionnaire de versions Node.js piloté par une interface web locale.
Il joue le même rôle que `nvm` / `fnm` / `volta`, mais toutes les actions se font depuis un dashboard dans le navigateur au lieu de la ligne de commande.

L'outil est un binaire Go unique et autonome.
Il démarre un petit serveur HTTP local, sert un dashboard, récupère la liste officielle des versions Node.js, permet de télécharger/activer/supprimer une version, et signale les mises à jour disponibles.

### Objectifs

- Installer et gérer plusieurs versions de Node.js sans avoir Node ni npm sur le système.
- Filtrer les versions par LTS / non-LTS.
- Voir d'un coup d'œil ce qui est installé, ce qui est actif, et ce qui est disponible.
- Changer la version active en un clic, de façon instantanée et réversible, sans droits root.
- Vérifier l'intégrité de chaque téléchargement (SHA256) avant installation.

### Non-objectifs (pour l'instant)

- Gestion par projet (fichier `.nvmrc` par dossier) : hors périmètre initial.
- Support multi-utilisateurs ou installation système globale (`/usr/local`) : `sono` travaille dans le home de l'utilisateur.
- Gestion de npm/pnpm/yarn séparément : ils viennent avec le tarball Node officiel.

## Choix de la stack et justification

| Élément | Choix | Raison |
| --- | --- | --- |
| Langage backend | Go (stdlib `net/http`) | Binaire unique, pas de runtime à installer, excellent pour manipuler archives et HTTP. |
| Front-end (rendu) | **htmx** + `html/template` (Go) | htmx est le choix de rendu du dashboard : le serveur Go produit le HTML via `html/template`, htmx pilote les mises à jour partielles du DOM. Pas de build front, pas de Node, pas de codegen (contrairement à `templ`). |
| Livraison de htmx | Vendorisé et embarqué (`go:embed`) | Pas de CDN : fonctionne hors-ligne, aucune dépendance réseau au runtime. |
| Dépendances tierces | Aucune (htmx = simple fichier statique) | Tout en stdlib Go ; htmx n'est pas une dépendance de build, juste un asset embarqué. Robustesse et maintenabilité long terme. |

### Pourquoi pas de Node dans la stack

Le but de l'outil est précisément d'installer Node sur une machine qui n'en a pas.
Utiliser un framework front qui exige Node pour être compilé (Svelte, React, etc.) créerait une dépendance circulaire au moment du build.
En rendant le HTML côté Go et en embarquant htmx comme simple fichier statique, `sono` se construit et fonctionne sans jamais avoir besoin de Node.

## Structure du projet

```
sono/
  go.mod                     # module sophonie/sono, go 1.26.5
  main.go                    # point d'entrée : parse les flags, démarre le serveur
  internal/
    config/                  # chemins ~/.sono, détection OS/arch, flags
    nodedist/                # client nodejs.org : index.json, SHASUMS256, download
    manager/                 # install / uninstall / set-active (symlink) / list installées
    server/                  # handlers HTTP, état des téléchargements en cours
  web/
    static/
      htmx.min.js            # vendorisé, embarqué via go:embed
      style.css
    templates/
      dashboard.html         # page principale
      *.html                 # partials rendus pour les réponses htmx
```

Les dossiers `web/static` et `web/templates` sont embarqués dans le binaire via `//go:embed`.
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

### `server`

- Sert le dashboard et les fichiers statiques embarqués.
- Expose les handlers d'action (installer, activer, supprimer).
- Suit l'état des téléchargements en cours (progression) dans une structure en mémoire protégée par mutex.
- Rend des fragments HTML pour les réponses htmx (mises à jour partielles du DOM).

## Flux d'installation

1. L'utilisateur clique sur « Télécharger » pour une version.
2. Le serveur lance l'installation dans une goroutine et enregistre un état de progression.
3. `nodedist` télécharge le tarball vers `cache/`, en calculant le SHA256 pendant l'écriture.
4. Le SHA256 calculé est comparé à l'entrée correspondante de `SHASUMS256.txt`.
   En cas de non-correspondance, le fichier est supprimé et l'installation échoue explicitement.
5. Le tarball vérifié est extrait dans `versions/v<X.Y.Z>/`.
6. Le dashboard reflète le nouvel état (polling htmx `hx-trigger="every 1s"` pendant le téléchargement).

La vérification du checksum est obligatoire et non contournable.

## Concurrence

- Chaque installation tourne dans sa propre goroutine.
- L'état des téléchargements (progression, erreurs) est stocké en mémoire dans le `server`, protégé par un `sync.Mutex`.
- Le repointage du symlink `current` est atomique (création d'un lien temporaire puis `rename`).

## Sécurité

- Le serveur écoute uniquement sur `127.0.0.1` (jamais exposé sur le réseau).
- Vérification SHA256 systématique avant toute extraction.
- Aucune commande shell n'est construite à partir d'entrées utilisateur ; les versions sont validées contre l'index officiel avant usage.
- `sono` n'a jamais besoin de droits root : tout se passe dans le home de l'utilisateur.

## Décisions d'architecture (résumé)

- `html/template` plutôt que `templ` : pas de codegen ni d'outil externe.
- htmx vendorisé plutôt que via CDN : fonctionnement hors-ligne et pas de dépendance réseau au runtime.
- Stdlib Go uniquement, aucune dépendance tierce : robustesse et maintenabilité long terme.
- Version active gérée par symlink : instantané, atomique, réversible, sans root.
