name: Bug Report
description: Report a bug in Breeze
title: "[Bug]: "
labels:
  - bug

body:
  - type: textarea
    id: description
    attributes:
      label: Describe the Bug
    validations:
      required: true

  - type: textarea
    id: reproduction
    attributes:
      label: Steps to Reproduce
    validations:
      required: true

  - type: input
    id: version
    attributes:
      label: Breeze Version

  - type: input
    id: goversion
    attributes:
      label: Go Version

  - type: textarea
    id: logs
    attributes:
      label: Additional Information