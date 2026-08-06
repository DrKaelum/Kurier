/// <reference path="./.sst/platform/config.d.ts" />

export default $config({
  app(input) {
    if (input.stage !== "viability") {
      throw new Error(
        `The SST spike only permits the "viability" stage; received "${input.stage}".`,
      );
    }

    return {
      name: "kurier-sst-spike",
      home: "aws",
      removal: "remove",
      protect: false,
      providers: {
        aws: {
          package: "@pulumi/aws",
          version: "7.40.0",
          region: "us-east-2",
        },
      },
    };
  },
  async run() {
    const vpc = new sst.aws.Vpc("ViabilityVpc", {
      az: 2,
    });
    const cluster = new sst.aws.Cluster("ViabilityCluster", { vpc });
    const queue = new sst.aws.Queue("ViabilityQueue", {
      visibilityTimeout: "30 seconds",
    });

    const queueAccess = new sst.Linkable("QueueAccess", {
      properties: {
        url: queue.url,
      },
      include: [
        sst.aws.permission({
          actions: ["sqs:SendMessage"],
          resources: [queue.arn],
        }),
      ],
    });

    const service = new sst.aws.Service("ViabilityService", {
      architecture: "arm64",
      capacity: "spot",
      cluster,
      cpu: "0.25 vCPU",
      memory: "0.5 GB",
      image: {
        context: "infra/spikes/sst-viability/service",
        dockerfile: "Dockerfile",
      },
      link: [queueAccess],
      environment: {
        KURIER_STAGE: $app.stage,
      },
      loadBalancer: {
        rules: [{ listen: "80/http", forward: "8080/http" }],
        health: {
          "8080/http": {
            path: "/health",
            interval: "15 seconds",
            healthyThreshold: 2,
          },
        },
      },
      logging: {
        retention: "1 week",
      },
      scaling: {
        min: 1,
        max: 1,
      },
    });

    const site = new sst.aws.StaticSite("ViabilityWeb", {
      path: "apps/web",
      build: {
        command: "npm run build",
        output: "dist",
      },
      environment: {
        VITE_KURIER_STAGE: $app.stage,
      },
    });

    return {
      region: "us-east-2",
      stage: $app.stage,
      serviceUrl: service.url,
      siteUrl: site.url,
      queueUrl: queue.url,
    };
  },
});
