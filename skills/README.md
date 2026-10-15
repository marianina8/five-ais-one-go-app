# Ep04 skills, installed unmodified

The skills tested in Ep04 ("5 Claude skills, tested"), copied byte for byte from their authors' repos at the commits below. Each folder keeps its author's license. The harness loads them with `--skills skills/<folder>`.

| Folder | Skill | Author | Source (commit) | License |
|---|---|---|---|---|
| superpowers | the whole pack (`skills/`) | Jesse Vincent | [obra/superpowers](https://github.com/obra/superpowers) @ bb92a77 | MIT |
| tdd | tdd | Matt Pocock | [mattpocock/skills](https://github.com/mattpocock/skills/tree/main/skills/engineering/tdd) @ 49dd158 | MIT |
| karpathy-guidelines | karpathy-guidelines | forrestchang | [forrestchang/andrej-karpathy-skills](https://github.com/forrestchang/andrej-karpathy-skills) @ 2c60614 | MIT (in its SKILL.md) |
| sharp-edges | sharp-edges | Trail of Bits | [trailofbits/skills](https://github.com/trailofbits/skills/tree/main/plugins/sharp-edges) @ e1f86a3 | CC BY-SA 4.0 |
| golang-pro | golang-pro | Jeff Allan | [jeffallan/claude-skills](https://github.com/jeffallan/claude-skills/tree/main/skills/golang-pro) @ 4f994f0 | MIT |
| go-pr-review | go-pr-provider (bonus) | Marian Montagnino | her own skill; the review itself runs the Ep01 agent's guidelines through `--review` | — |

The bonus skill's procedure runs shell scripts and git, which the contest harness doesn't offer, so in the bonus arm the harness also turns on `request_review`: the Ep01 review agent (`harness/review_guidelines.md`, from [marianina8/pr-review-agent](https://github.com/marianina8/pr-review-agent)) reviewing the workspace with read-only tools.
