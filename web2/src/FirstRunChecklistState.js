export function getFirstRunChecklist({account, signinOptions, machines, stats}) {
  const passwordDone = Boolean(
    account &&
      (account.owner !== "basic" ||
        signinOptions?.signinAvailable === false ||
        signinOptions?.autoSignin === false)
  );

  return [
    {key: "password", done: passwordDone},
    {key: "machine", done: Array.isArray(machines) && machines.length > 0},
    {key: "node", done: Number(stats?.nodesReady) > 0},
    {key: "app", done: Number(stats?.deploymentsTotal) > 0},
  ];
}
