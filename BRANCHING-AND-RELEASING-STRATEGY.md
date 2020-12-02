# Branching and releasing strategy of CB-Larva

기본적인 설명은 Cloud-Barista's Coffeehouse의 [Git branching and releasing strategy](https://github.com/cb-contributhon/cb-coffeehouse/wiki/Git-branching-strategy-and-release-management)를 참고 바랍니다.

## Branching strategy of CB-Larva
- `master` : 제품으로 출시될 수 있는 브랜치
- `develop` : 다음 출시 버전을 개발하는 브랜치
- `feature` : 기능을 개발하는 브랜치
- `release` : 이번 출시 버전을 준비하는 브랜치
- `hotfix` : 출시 버전에서 발생한 버그를 수정 하는 브랜치

## Releasing strategy of CB-Larva
CB-Larva는 Cloud-Barista의 기술 인큐베이터로서 주로 개념증명(POC, Proof of concept)을 수행합니다. 여러 기술에 대한 POC를 진행하므로 이에 대한 **별도의 Release strategy를 적용** 하고자 합니다.

### 1. 배포 준비를 위한 브랜치 생성
아래 Branch naming convention을 준수하여 배포 준비를 위한 `release` 브랜치를 생성합니다. 버전은 배포할 버전으로 설정 합니다. 
- Branch naming convention: **`release-TECHNAME-MAJOR.MINOR.PATCH`** (예, `release-cb-network-0.0.1`, `release-cb-storage-0.0.1`)

예를 들어, cb-network의 이전 배포 버전이 0.0.6(master branch의 tag 기준)이었고, 이번에 0.1.0을 배포할 것이라면, `release-cb-network-0.1.0` 브랜치를 생성합니다.

### 2. 생성한 `release` 브랜치에서 배포 준비작업 수행
`release` 브랜치에서는 **오직 Bugfixes만을 수행**합니다. ***<ins>신규 기능을 개발하지 않습니다.</ins>***

Bugfixes는 지속적으로 `develop` 브랜치로 머지합니다.

배포 준비가 완료되면 `develop` 브랜치와 `master` 브랜치에 머지합니다.

### 3. 배포
아래 Tag naming convention을 준수하여 `master` 브랜치에 Tagging을 합니다. `-m` 옵션을 사용하면 commit과 마찬가지로 tagging시 제목과 설명을 입력할 수 있습니다. 
- Tag naming convention: **`TECHNAME-MAJOR.MINOR.PATCH`** (예, `cb-network-0.0.1`, `cb-storage-0.0.1`)

#### 참고사항 
**"GitHub의 Releases"** 는 웹페이지 상에서 "3.배포" 단계를 수행합니다. (`release` 브랜치와는 다릅니다. 저도 처음에 잘못 이해했어요 :sob:) 

Tagging, Release title, Description을 한 페이지에서 수행할 수 있습니다.

자세한 과정은 [Managing releases in a repository](https://docs.github.com/en/free-pro-team@latest/github/administering-a-repository/managing-releases-in-a-repository)을 참고 바랍니다.


### 4. 배포 후에 발생한 문제 처리
아래 Hotfix naming convention을 준수하여 `hotfix` 브랜치를 생성한 후 신속하게 문제를 처리합니다. 처리 후에는 `develop` 브랜치와 `master` 브랜치에 머지 합니다.

- Hotfix naming convention: **`hotfix-TECHNAME-MAJOR.MINOR.PATCH`** (예, `hotfix-cb-network-0.0.1`, `hotfix-cb-storage-0.0.1`)

**<ins>"3. 배포" 과정을 꼭! 수행합니다.</ins>** (마지막 버전 +1 예, 0.1.0 -> 0.1.1)