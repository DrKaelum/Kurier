/// <reference path="./.sst/platform/config.d.ts" />

const allowedStages = ["viability", "db-viability"] as const;

export default $config({
  app(input) {
    if (
      !allowedStages.includes(input.stage as (typeof allowedStages)[number])
    ) {
      throw new Error(
        `The SST spikes permit only ${allowedStages.map((stage) => `"${stage}"`).join(" or ")}; received "${input.stage}".`,
      );
    }

    return {
      name:
        input.stage === "db-viability"
          ? "kurier-sst-db-spike"
          : "kurier-sst-spike",
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
    if ($app.stage === "db-viability") {
      return await createDatabaseViabilitySpike();
    }
    return createNonDatabaseViabilitySpike();
  },
});

function createNonDatabaseViabilitySpike() {
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
}

async function createDatabaseViabilitySpike() {
  const { output } = await import("@pulumi/pulumi");
  const availabilityZones = aws.getAvailabilityZonesOutput({
    state: "available",
  }).names;
  const vpc = new aws.ec2.Vpc("DatabaseViabilityVpc", {
    cidrBlock: "10.0.0.0/16",
    enableDnsHostnames: true,
    enableDnsSupport: true,
    tags: {
      Name: "kurier-db-viability",
    },
  });
  const internetGateway = new aws.ec2.InternetGateway(
    "DatabaseViabilityInternetGateway",
    {
      tags: { Name: "kurier-db-viability" },
      vpcId: vpc.id,
    },
  );
  const publicRouteTable = new aws.ec2.RouteTable(
    "DatabaseViabilityPublicRouteTable",
    {
      routes: [
        {
          cidrBlock: "0.0.0.0/0",
          gatewayId: internetGateway.id,
        },
      ],
      tags: { Name: "kurier-db-viability-public" },
      vpcId: vpc.id,
    },
  );
  const privateRouteTable = new aws.ec2.RouteTable(
    "DatabaseViabilityPrivateRouteTable",
    {
      tags: { Name: "kurier-db-viability-private" },
      vpcId: vpc.id,
    },
  );
  const publicSubnets = [0, 1].map(
    (index) =>
      new aws.ec2.Subnet(`DatabaseViabilityPublicSubnet${index + 1}`, {
        availabilityZone: availabilityZones.apply((zones) => zones[index]),
        cidrBlock: `10.0.${index}.0/24`,
        mapPublicIpOnLaunch: true,
        tags: { Name: `kurier-db-viability-public-${index + 1}` },
        vpcId: vpc.id,
      }),
  );
  const privateSubnets = [0, 1].map(
    (index) =>
      new aws.ec2.Subnet(`DatabaseViabilityPrivateSubnet${index + 1}`, {
        availabilityZone: availabilityZones.apply((zones) => zones[index]),
        cidrBlock: `10.0.${index + 10}.0/24`,
        mapPublicIpOnLaunch: false,
        tags: { Name: `kurier-db-viability-private-${index + 1}` },
        vpcId: vpc.id,
      }),
  );
  publicSubnets.forEach((subnet, index) => {
    new aws.ec2.RouteTableAssociation(
      `DatabaseViabilityPublicRouteTableAssociation${index + 1}`,
      {
        routeTableId: publicRouteTable.id,
        subnetId: subnet.id,
      },
    );
  });
  privateSubnets.forEach((subnet, index) => {
    new aws.ec2.RouteTableAssociation(
      `DatabaseViabilityPrivateRouteTableAssociation${index + 1}`,
      {
        routeTableId: privateRouteTable.id,
        subnetId: subnet.id,
      },
    );
  });
  const serviceSecurityGroup = new aws.ec2.SecurityGroup(
    "DatabaseViabilityServiceSecurityGroup",
    {
      description: "Database viability service traffic",
      egress: [
        {
          cidrBlocks: ["169.254.170.2/32"],
          fromPort: 80,
          protocol: "tcp",
          toPort: 80,
        },
        {
          cidrBlocks: ["0.0.0.0/0"],
          fromPort: 443,
          protocol: "tcp",
          toPort: 443,
        },
        {
          cidrBlocks: ["10.0.0.2/32"],
          fromPort: 53,
          protocol: "tcp",
          toPort: 53,
        },
        {
          cidrBlocks: ["10.0.0.2/32"],
          fromPort: 53,
          protocol: "udp",
          toPort: 53,
        },
        {
          cidrBlocks: ["10.0.0.0/16"],
          fromPort: 5432,
          protocol: "tcp",
          toPort: 5432,
        },
      ],
      ingress: [],
      tags: { Name: "kurier-db-viability-service" },
      vpcId: vpc.id,
    },
  );
  const databaseSecurityGroup = new aws.ec2.SecurityGroup(
    "DatabaseViabilityDatabaseSecurityGroup",
    {
      description: "PostgreSQL access from the database viability service only",
      vpcId: vpc.id,
      ingress: [
        {
          fromPort: 5432,
          protocol: "tcp",
          securityGroups: [serviceSecurityGroup.id],
          toPort: 5432,
        },
      ],
      egress: [],
      tags: {
        Name: "kurier-db-viability-database",
      },
    },
  );

  const database = new sst.aws.Postgres("DatabaseViabilityPostgres", {
    database: "kurier_db_viability",
    instance: "t4g.micro",
    multiAz: false,
    storage: "20 GB",
    version: "17.10",
    vpc: {
      subnets: privateSubnets.map((subnet) => subnet.id),
    },
    transform: {
      instance(args) {
        args.backupRetentionPeriod = 1;
        args.deleteAutomatedBackups = true;
        args.deletionProtection = false;
        args.identifier = "kurier-sst-db-spike-db-viability-postgres";
        args.maxAllocatedStorage = 0;
        args.performanceInsightsEnabled = false;
        args.publiclyAccessible = false;
        args.skipFinalSnapshot = true;
        args.storageEncrypted = true;
        args.vpcSecurityGroupIds = [databaseSecurityGroup.id];
      },
      parameterGroup(args) {
        args.parameters = [
          {
            applyMethod: "immediate",
            name: "rds.force_ssl",
            value: "1",
          },
        ];
      },
    },
  });

  const databaseSecretArn = database.nodes.instance.tagsAll.apply((tags) => {
    const secretArn = tags["sst:lookup:password"];
    if (!secretArn) {
      throw new Error(
        "SST did not expose its generated PostgreSQL secret ARN.",
      );
    }
    return secretArn;
  });

  const databaseAccess = new sst.Linkable("DatabaseAccess", {
    properties: {
      database: database.database,
      host: database.host,
      port: database.port,
      secretArn: databaseSecretArn,
      username: database.username,
    },
  });

  const cluster = new sst.aws.Cluster("DatabaseViabilityCluster", {
    vpc: {
      containerSubnets: publicSubnets.map((subnet) => subnet.id),
      id: vpc.id,
      loadBalancerSubnets: publicSubnets.map((subnet) => subnet.id),
      securityGroups: [serviceSecurityGroup.id],
    },
  });
  const service = new sst.aws.Service("DatabaseViabilityService", {
    architecture: "arm64",
    capacity: "spot",
    cluster,
    cpu: "0.25 vCPU",
    memory: "0.5 GB",
    image: {
      context: "infra/spikes/sst-db-viability/service",
      dockerfile: "Dockerfile",
    },
    link: [databaseAccess],
    environment: {
      AWS_REGION: "us-east-2",
      KURIER_STAGE: $app.stage,
      RDS_CA_BUNDLE: "/etc/ssl/certs/rds-global-bundle.pem",
    },
    loadBalancer: {
      rules: [{ listen: "80/http", forward: "8080/http" }],
      health: {
        "8080/http": {
          path: "/service-health",
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
    transform: {
      executionRole(args) {
        args.inlinePolicies = [];
      },
      loadBalancerSecurityGroup(args) {
        args.ingress = [
          {
            cidrBlocks: ["0.0.0.0/0"],
            fromPort: 80,
            protocol: "tcp",
            toPort: 80,
          },
        ];
      },
      service(args) {
        args.enableExecuteCommand = false;
        args.networkConfiguration = output(args.networkConfiguration).apply(
          (networkConfiguration) => ({
            ...networkConfiguration,
            assignPublicIp: true,
          }),
        );
      },
      taskRole(args) {
        args.inlinePolicies = [
          {
            name: "read-database-secret",
            policy: aws.iam.getPolicyDocumentOutput({
              statements: [
                {
                  actions: ["secretsmanager:GetSecretValue"],
                  resources: [databaseSecretArn],
                },
              ],
            }).json,
          },
        ];
      },
    },
    wait: true,
  });
  new aws.ec2.SecurityGroupRule("DatabaseViabilityAlbToServiceIngress", {
    fromPort: 8080,
    protocol: "tcp",
    securityGroupId: serviceSecurityGroup.id,
    sourceSecurityGroupId: service.nodes.loadBalancer.securityGroups.apply(
      (securityGroups) => securityGroups[0],
    ),
    toPort: 8080,
    type: "ingress",
  });

  return {
    databaseId: database.id,
    region: "us-east-2",
    serviceUrl: service.url,
    stage: $app.stage,
  };
}
