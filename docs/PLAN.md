# Plan de construction - `sono`

Ce document détaille la construction incrémentale de `sono`, phase par phase.
Chaque phase est autonome, vérifiable, et laisse le projet dans un état fonctionnel.
Voir [ARCHITECTURE.md](./ARCHITECTURE.md) pour les décisions de conception sous-jacentes.

## Conventions

- Module Go : `sophonie/sono`, Go 1.26.5.
- Aucune dépendance tierce : stdlib uniquement.
- Racine des données : `~/.sono/`.
- Serveur d'écoute par défaut : `127.0.0.1:8420` (surchargable par flag `-addr`).
- Chaque phase se termine par une étape de vérification explicite (build + test manuel).

## Phase 1 - Squelette

Objectif : un binaire qui démarre, crée son arborescence, et sert une page vide.

Tâches :

1. `internal/config/config.go`
   - Résoudre `~/.sono` (via `os.UserHomeDir`).
   - Créer `versions/` et `cache/` si absents.
   - Exposer les chemins : `Root`, `VersionsDir`, `CacheDir`, `CurrentSymlink`, `IndexCache`.
   - Détecter OS/arch et fournir `Platform()` (ex. `linux-x64`).
2. `internal/server/server.go`
   - Serveur `net/http` lié à `127.0.0.1:8420`.
   - Route `GET /` rendant un `dashboard.html` minimal.
   - Route statique `GET /static/` servant les assets embarqués.
3. `web/templates/dashboard.html` : page minimale (titre, conteneur vide).
4. `web/static/` : ajouter `style.css` de base et `htmx.min.js` (vendorisé).
5. `main.go` : parse le flag `-addr`, initialise `config`, démarre le serveur, affiche l'URL.

Vérification :

- `go build ./...` réussit.
- `go vet ./...` est propre.
- Lancer le binaire, ouvrir `http://127.0.0.1:8420`, la page s'affiche.
- `~/.sono/versions` et `~/.sono/cache` sont créés.

## Phase 2 - Dashboard en lecture seule

Objectif : afficher la liste des versions (disponibles, installées, active) avec filtres.

Tâches :

1. `internal/nodedist/index.go`
   - Structs correspondant à `index.json` (`version`, `date`, `lts`, `files`).
   - `FetchIndex()` : télécharge `index.json`, met en cache dans `~/.sono/index.json`, avec TTL.
   - `LoadIndex()` : lit le cache local si frais, sinon refetch.
   - Filtres : `LTS()`, `NonLTS()`, recherche par préfixe de version.
2. `internal/manager/manager.go`
   - `ListInstalled()` : scan de `~/.sono/versions`.
   - `Active()` : `readlink` de `current`, renvoie la version active ou vide.
   - `ResolvedOnPath()` : résout `node` sur le `PATH` réel (best effort) pour comparaison.
3. `internal/server` : handler `GET /` enrichi.
   - Assemble un view-model : versions installées, active, PATH réel, versions dispo filtrées.
   - Filtres via query params (`?filter=lts|nonlts|all&q=<recherche>`).
   - Fragment htmx `GET /versions` pour rafraîchir la liste sans recharger la page.
4. `web/templates` : tableau des versions, badges LTS, indicateur « active », barre de filtres/recherche htmx.

Vérification :

- La liste des versions se charge depuis nodejs.org (puis depuis le cache).
- Les filtres LTS / non-LTS / toutes fonctionnent.
- La recherche filtre correctement.
- Une version installée manuellement dans `~/.sono/versions` apparaît comme installée.

## Phase 3 - Installation

Objectif : télécharger, vérifier et extraire une version depuis le dashboard.

Tâches :

1. `internal/nodedist/download.go`
   - `FetchChecksums(version)` : parse `SHASUMS256.txt`.
   - `Download(version, platform, dest, progress)` : télécharge le tarball en streaming, calcule le SHA256 au vol, appelle un callback de progression.
2. `internal/manager` : `Install(version)`.
   - Construit l'URL et le nom de fichier attendu pour la plateforme courante.
   - Télécharge vers `cache/`, vérifie le SHA256 contre `SHASUMS256.txt`.
   - En cas d'échec de checksum : supprime le fichier, renvoie une erreur explicite.
   - Extrait le `.tar.xz` (via `archive/tar` + décompression xz) dans `versions/v<X.Y.Z>/`.
3. `internal/server`
   - `POST /install?version=<v>` : lance l'install en goroutine, enregistre l'état.
   - Structure d'état des téléchargements en mémoire (mutex).
   - `GET /install/status?version=<v>` : fragment htmx de progression, poll `every 1s`.
4. `web/templates` : bouton « Télécharger » par ligne, barre de progression htmx.

Point d'attention : décompression xz.
Vérifier d'abord si la stdlib suffit ; sinon, comparer les options (télécharger le `.tar.gz` à la place, ou ajouter une lib xz) et proposer le choix avant de coder.

Vérification :

- Installer une version LTS de bout en bout depuis le dashboard.
- Le SHA256 est vérifié (tester qu'un checksum falsifié fait échouer proprement).
- Le dossier `versions/v<X.Y.Z>/bin/node` existe après extraction.
- La progression s'affiche puis disparaît à la fin.

## Phase 4 - Gestion (activer, supprimer, PATH)

Objectif : rendre une version active, en supprimer, aider à configurer le `PATH`.

Tâches :

1. `internal/manager`
   - `SetActive(version)` : repointe `current` de façon atomique (symlink temporaire + `rename`).
   - `Uninstall(version)` : supprime le dossier ; refuse si c'est la version active (ou avertit et détache d'abord).
2. `internal/server`
   - `POST /activate?version=<v>`.
   - `POST /uninstall?version=<v>`.
   - Handlers renvoyant le fragment de liste mis à jour (htmx).
3. `web/templates`
   - Boutons « Activer » et « Supprimer » par ligne.
   - Bandeau d'aide PATH : afficher la ligne exacte à ajouter au `.bashrc` (`export PATH="$HOME/.sono/current/bin:$PATH"`), avec bouton copier.
   - Avertissement visible si la version active du symlink diffère de celle du `PATH` réel.

Vérification :

- Activer une version repointe le symlink ; `~/.sono/current/bin/node --version` renvoie la bonne version.
- Après ajout au `PATH` et nouveau shell, `node -v` renvoie la version active.
- Supprimer une version non active fonctionne ; supprimer l'active est bloqué/averti.

## Phase 5 - Mises à jour

Objectif : signaler les versions installées qui ont un patch plus récent disponible.

Tâches :

1. `internal/manager` : `AvailableUpdates()`.
   - Pour chaque version installée, chercher le dernier patch de la même ligne mineure dans l'index.
   - Signaler aussi le dernier LTS disponible globalement.
2. `internal/server` + `web/templates`
   - Badge « mise à jour dispo » sur les lignes concernées.
   - Action « Mettre à jour » = installer la version cible puis proposer de l'activer.

Vérification :

- Une version installée volontairement ancienne fait apparaître un badge de mise à jour.
- Le bouton « Mettre à jour » installe la version cible correctement.

## Suites possibles (hors périmètre initial)

- Purge du cache des tarballs depuis l'UI.
- Gestion par projet via un fichier de version.
- Ouverture automatique du navigateur au démarrage.
- Empaquetage/installation du binaire `sono` lui-même (service, autostart).

## Ordre de livraison

Les phases 1 et 2 sont livrées d'abord ensemble, pour avoir vite un dashboard concret et visible à l'écran.
Les phases 3 à 5 suivent une fois la lecture seule validée.
