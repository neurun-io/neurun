// @novnc/novnc declares its types on the /lib/rfb subpath while its exports map
// only permits the bare specifier at runtime. This bridges the two.
declare module "@novnc/novnc" {
  import RFB from "@novnc/novnc/lib/rfb";
  export default RFB;
}
