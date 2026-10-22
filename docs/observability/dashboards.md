UI updates are disallowed in Grafana since dashboards are stored as json in `infra/k8s/grafana/dashboards`. If changes were to be made in UI, then they would be overwritten with what is currently saved whenever the grafana service restarts.

When you want to make changes and click save on a panel or dashboard, Grafana will tell you to export the updated state as json. copy/download it and replace whatever dashboard you are working on inside the directory.
